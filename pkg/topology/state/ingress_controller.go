package state

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/hashicorp/go-version"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Supported Ingress Controllers.
const (
	IngressControllerTraefik        = "traefik"
	IngressControllerNginx          = "nginx"
	IngressControllerNginxCommunity = "nginx-community"
)

func (f *Fetcher) getIngressControllers(services map[string]*Service) (map[string]*IngressController, error) {
	pods, err := f.k8s.Core().V1().Pods().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	// Sort pods to ensure that we always pick the same one and avoid producing a diff over and over.
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Status.PodIP < pods[j].Status.PodIP
	})

	result := make(map[string]*IngressController)
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		controller := podHasIngressController(pod)
		if controller == "" {
			continue
		}

		// TODO Support multiple Ingress controllers of the same type in the same namespace
		key := objectKey(controller, pod.Namespace)

		service := findServiceForPodLabels(services, pod.Labels)
		if service == nil {
			return nil, fmt.Errorf("find service for ingress controller %s", key)
		}

		var replicas int
		replicas, err = f.getReplicas(pod)
		if err != nil {
			return nil, err
		}

		if ic, exists := result[key]; exists {
			ic.MetricsURLs = append(ic.MetricsURLs, guessMetricsURL(controller, pod))
			continue
		}

		result[key] = &IngressController{
			Name:             controller,
			Namespace:        pod.Namespace,
			MetricsURLs:      []string{guessMetricsURL(controller, pod)},
			PublicIPs:        ingressesToIPs(service.Status.LoadBalancer.Ingress),
			ServiceAddresses: service.addresses,
			Replicas:         replicas,
		}
	}

	// Stop early if no IngressController was found.
	if len(result) == 0 {
		return result, nil
	}

	// Stop early if server does not support IngressClasses.
	if f.serverVersion.LessThan(version.Must(version.NewVersion("1.18"))) {
		return result, nil
	}

	ingressClasses, err := f.fetchIngressClasses()
	if err != nil {
		return nil, err
	}

	setIngressClasses(result, ingressClasses)

	return result, nil
}

func (f *Fetcher) fetchIngressClasses() ([]*netv1.IngressClass, error) {
	ingressClasses, err := f.k8s.Networking().V1().IngressClasses().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	if f.serverVersion.GreaterThanOrEqual(version.Must(version.NewVersion("1.19"))) {
		return ingressClasses, nil
	}

	v1beta1IngressClasses, err := f.k8s.Networking().V1beta1().IngressClasses().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, ingressClass := range v1beta1IngressClasses {
		networking, err := marshalToIngressClassNetworkingV1(ingressClass)
		if err != nil {
			return nil, err
		}

		ingressClasses = append(ingressClasses, networking)
	}

	return ingressClasses, nil
}

func setIngressClasses(result map[string]*IngressController, ingressClasses []*netv1.IngressClass) {
	// Ensure the order of IngressClasses always remains the same.
	sort.Slice(ingressClasses, func(i, j int) bool {
		return ingressClasses[i].Name < ingressClasses[j].Name
	})

	// TODO: Support custom controller values.
	// TODO: Detect which ingress class is selected by which controller.
	for _, ingressClass := range ingressClasses {
		switch ingressClass.Spec.Controller {
		case "k8s.io/ingress-nginx":
			for _, controller := range result {
				if controller.Name == IngressControllerNginxCommunity {
					controller.IngressClasses = append(controller.IngressClasses, ingressClass.Name)
				}
			}
		case "nginx.org/ingress-controller":
			for _, controller := range result {
				if controller.Name == IngressControllerNginx {
					controller.IngressClasses = append(controller.IngressClasses, ingressClass.Name)
				}
			}
		case "traefik.io/ingress-controller":
			for _, controller := range result {
				if controller.Name == IngressControllerTraefik {
					controller.IngressClasses = append(controller.IngressClasses, ingressClass.Name)
				}
			}
		}
	}
}

func (f *Fetcher) getReplicas(pod *corev1.Pod) (int, error) {
	var owner metav1.OwnerReference
	for _, o := range pod.ObjectMeta.OwnerReferences {
		if o.Controller == nil || !(*o.Controller) {
			continue
		}

		owner = o
		break
	}

	switch owner.Kind {
	case "Deployment":
		deployment, err := f.k8s.Apps().V1().Deployments().Lister().Deployments(pod.Namespace).Get(owner.Name)
		if err != nil {
			return 0, err
		}

		return int(*deployment.Spec.Replicas), nil

	case "StatefulSet":
		statefulSet, err := f.k8s.Apps().V1().StatefulSets().Lister().StatefulSets(pod.Namespace).Get(owner.Name)
		if err != nil {
			return 0, err
		}

		return int(*statefulSet.Spec.Replicas), nil

	case "ReplicaSet":
		replicaSet, err := f.k8s.Apps().V1().ReplicaSets().Lister().ReplicaSets(pod.Namespace).Get(owner.Name)
		if err != nil {
			return 0, err
		}

		return int(*replicaSet.Spec.Replicas), nil

	case "DaemonSet":
		daemonSet, err := f.k8s.Apps().V1().DaemonSets().Lister().DaemonSets(pod.Namespace).Get(owner.Name)
		if err != nil {
			return 0, err
		}

		return int(daemonSet.Status.DesiredNumberScheduled), nil
	}

	return 0, errors.New("no controller owner found for this pod")
}

// guessMetricsURL builds the metrics endpoint URL based on simple assumptions for a given pod.
// For instance, this will not work if someone use a specific configuration to expose the prometheus metrics endpoint.
// TODO we can try to use the IngressController configuration to be more accurate.
func guessMetricsURL(ctrl string, pod *corev1.Pod) string {
	// Metrics are not supported for Nginx official.
	if ctrl == IngressControllerNginx {
		return ""
	}

	var port string
	switch ctrl {
	case IngressControllerTraefik:
		port = "8080"
	case IngressControllerNginxCommunity:
		port = "10254"
	}

	if pod.Annotations["prometheus.io/port"] != "" {
		port = pod.Annotations["prometheus.io/port"]
	}

	path := "metrics"
	if pod.Annotations["prometheus.io/path"] != "" {
		path = pod.Annotations["prometheus.io/path"]
	}

	return fmt.Sprintf("http://%s/%s", net.JoinHostPort(pod.Status.PodIP, port), path)
}

func podHasIngressController(pod *corev1.Pod) string {
	for _, container := range pod.Spec.Containers {
		if strings.Contains(container.Image, "traefik:") {
			return IngressControllerTraefik
		}

		if strings.Contains(container.Image, "nginx/nginx-ingress:") {
			return IngressControllerNginx
		}

		if strings.Contains(container.Image, "ingress-nginx/controller:") {
			return IngressControllerNginxCommunity
		}
	}

	return ""
}

func findServiceForPodLabels(svcs map[string]*Service, lbls map[string]string) *Service {
	for _, service := range svcs {
		if len(service.Status.LoadBalancer.Ingress) == 0 {
			continue
		}

		var match bool
		for sKey, sVal := range service.Selector {
			if lbls[sKey] != sVal {
				match = false
				break
			}
			match = true
		}

		if match {
			return service
		}
	}

	return nil
}

func ingressesToIPs(ingresses []corev1.LoadBalancerIngress) []string {
	var ips []string
	for _, ingress := range ingresses {
		ips = append(ips, ingress.IP)
	}

	sort.Strings(ips)

	return ips
}

func marshalToIngressClassNetworkingV1(ing *v1beta1.IngressClass) (*netv1.IngressClass, error) {
	data, err := ing.Marshal()
	if err != nil {
		return nil, err
	}

	ni := &netv1.IngressClass{}
	err = ni.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	return ni, nil
}
