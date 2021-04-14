package state

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/hashicorp/go-version"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
)

// Supported Ingress Controllers.
// TODO: unify constants with ACP.
const (
	ControllerTypeNginxOfficial    = "nginx.org/ingress-controller"
	ControllerTypeNginxCommunity   = "k8s.io/ingress-nginx"
	ControllerTypeHAProxyCommunity = "haproxy-ingress.github.io/controller"
	ControllerTypeTraefik          = "traefik.io/ingress-controller"
)

// Supported Ingress controller types.
const (
	IngressControllerTypeTraefik          = "traefik"
	IngressControllerTypeNginxOfficial    = "nginx"
	IngressControllerTypeNginxCommunity   = "nginx-community"
	IngressControllerTypeHAProxyCommunity = "haproxy-community"
)

func (f *Fetcher) getIngressControllers(services map[string]*Service, apps map[string]*App) (map[string]*IngressController, error) {
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

		controller := getControllerType(pod)
		if controller == "" {
			continue
		}

		app := findApp(apps, pod)
		key := objectKey(app.Name, app.Namespace)

		ic, exists := result[key]
		if !exists {
			ic = &IngressController{
				App:  app,
				Type: controller,

				// TODO What should we do if an IngressController does not have a service, log, status field?
				PublicIPs: findPublicIPs(services, pod),
			}

			result[key] = ic
		}

		metricsURL := guessMetricsURL(controller, pod)
		if metricsURL != "" {
			ic.MetricsURLs = append(ic.MetricsURLs, metricsURL)
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

func setIngressClasses(controllers map[string]*IngressController, ingressClasses []*netv1.IngressClass) {
	// Ensure the order of IngressClasses always remains the same.
	sort.Slice(ingressClasses, func(i, j int) bool {
		return ingressClasses[i].Name < ingressClasses[j].Name
	})

	// TODO: Support custom controller values.
	// TODO: Detect which ingress class is selected by which controller.
	for _, ingressClass := range ingressClasses {
		var ctrlType string

		switch ingressClass.Spec.Controller {
		case ControllerTypeNginxCommunity:
			ctrlType = IngressControllerTypeNginxCommunity

		case ControllerTypeNginxOfficial:
			ctrlType = IngressControllerTypeNginxOfficial

		case ControllerTypeTraefik:
			ctrlType = IngressControllerTypeTraefik

		case ControllerTypeHAProxyCommunity:
			ctrlType = IngressControllerTypeHAProxyCommunity

		default:
			continue
		}

		for _, controller := range controllers {
			if controller.Type == ctrlType {
				controller.IngressClasses = append(controller.IngressClasses, ingressClass.Name)
			}
		}
	}
}

// guessMetricsURL builds the metrics endpoint URL based on simple assumptions for a given pod.
// For instance, this will not work if someone use a specific configuration to expose the prometheus metrics endpoint.
// TODO we can try to use the IngressController configuration to be more accurate.
func guessMetricsURL(ctrl string, pod *corev1.Pod) string {
	// Metrics are not supported for Nginx official.
	if ctrl == IngressControllerTypeNginxOfficial {
		return ""
	}

	var port string
	switch ctrl {
	case IngressControllerTypeTraefik:
		port = "8080"
	case IngressControllerTypeNginxCommunity:
		port = "10254"
	case IngressControllerTypeHAProxyCommunity:
		port = "9101"
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

func getControllerType(pod *corev1.Pod) string {
	for _, container := range pod.Spec.Containers {
		// A container image has the form image:tag and the tag is optional.
		parts := strings.Split(container.Image, ":")

		switch parts[0] {
		case "traefik":
			return IngressControllerTypeTraefik

		case "nginx/nginx-ingress":
			return IngressControllerTypeNginxOfficial

		case "k8s.gcr.io/ingress-nginx/controller":
			return IngressControllerTypeNginxCommunity

		case "quay.io/jcmoraisjr/haproxy-ingress":
			return IngressControllerTypeHAProxyCommunity
		}
	}

	return ""
}

// TODO: Check if this might cause issues with multiple apps with the same pod labels, or if it is an expected config error.
func findApp(apps map[string]*App, pod *corev1.Pod) App {
	var result []App
	for _, app := range apps {
		if app.Namespace != pod.Namespace {
			continue
		}

		var match bool
		for sKey, sVal := range app.podLabels {
			if pod.Labels[sKey] != sVal {
				match = false
				break
			}
			match = true
		}

		if match {
			result = append(result, *app)
		}
	}

	if len(result) == 0 {
		return App{}
	}

	// In case the pod matches multiple apps we have to pick one.
	// As we don't know which one to pick we have to sort the matching apps before taking the first one.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result[0]
}

func findPublicIPs(svcs map[string]*Service, pod *corev1.Pod) []string {
	var ips []string

	knownIPs := make(map[string]struct{})
	for _, service := range svcs {
		if service.Namespace != pod.Namespace || len(service.status.LoadBalancer.Ingress) == 0 {
			continue
		}

		var match bool
		for sKey, sVal := range service.Selector {
			if pod.Labels[sKey] != sVal {
				match = false
				break
			}
			match = true
		}

		if !match {
			continue
		}

		for _, ip := range service.status.LoadBalancer.Ingress {
			if _, exists := knownIPs[ip.IP]; exists {
				continue
			}

			knownIPs[ip.IP] = struct{}{}
			ips = append(ips, ip.IP)
		}
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
