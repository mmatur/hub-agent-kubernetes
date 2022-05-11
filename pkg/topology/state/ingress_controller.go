package state

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/kubevers"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
)

// AnnotationHubIngressController is the annotation to add to a Deployment/ReplicaSet/StatefulSet/DaemonSet to specify the Ingress controller type.
const AnnotationHubIngressController = "hub.traefik.io/ingress-controller"

// Supported Ingress controller types.
const (
	IngressControllerTypeNone             = "none"
	IngressControllerTypeTraefik          = "traefik"
	IngressControllerTypeHAProxyCommunity = "haproxy-community"
)

// Supported Ingress Controllers.
// TODO: unify constants with ACP.
const (
	ControllerTypeHAProxyCommunity = "haproxy-ingress.github.io/controller"
	ControllerTypeTraefik          = "traefik.io/ingress-controller"
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

		var ctrlType string
		ctrlType, err = f.getIngressControllerType(pod)
		if err != nil {
			return nil, err
		}
		if !isSupportedIngressControllerType(ctrlType) {
			log.Error().Msgf("Ingress controller type %s is not supported", ctrlType)
			continue
		}
		if ctrlType == IngressControllerTypeNone {
			continue
		}

		app := findApp(apps, pod)
		key := objectKey(app.Name, app.Namespace)

		ic, exists := result[key]
		if !exists {
			ic = &IngressController{
				App:  app,
				Type: ctrlType,

				// TODO What should we do if an IngressController does not have a service, log, status field?
				PublicEndpoints: findPublicEndpoints(services, pod),
				Endpoints:       findEndpoints(services, pod),
			}

			result[key] = ic
		}

		metricsURL := guessMetricsURL(ctrlType, pod)
		if metricsURL != "" {
			ic.MetricsURLs = append(ic.MetricsURLs, metricsURL)
		}
	}

	// Stop early if no IngressController was found.
	if len(result) == 0 {
		return result, nil
	}

	// Stop early if server does not support IngressClasses.
	if !kubevers.SupportsIngressClasses(f.serverVersion) {
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

func (f *Fetcher) getIngressControllerType(pod *corev1.Pod) (string, error) {
	value, err := f.getAnnotation(pod, AnnotationHubIngressController)
	if err != nil {
		return "", fmt.Errorf("get %s annotation: %w", AnnotationHubIngressController, err)
	}
	if value != "" {
		return value, nil
	}

	for _, container := range pod.Spec.Containers {
		// A container image has the form image:tag and the tag is optional.
		parts := strings.Split(container.Image, ":")

		if strings.HasSuffix(parts[0], "traefik") {
			return IngressControllerTypeTraefik, nil
		}

		// For now we are only detecting TraefikEE proxies to be able to fetch metrics.
		if strings.HasSuffix(parts[0], "traefikee") && contains(container.Command, "proxy") {
			return IngressControllerTypeTraefik, nil
		}

		if strings.Contains(parts[0], "jcmoraisjr/haproxy-ingress") {
			return IngressControllerTypeHAProxyCommunity, nil
		}
	}

	return IngressControllerTypeNone, nil
}

// getAnnotation returns the value for the given annotation key from the given pod.
// If the annotation is not found on the pod, the annotation value will be retrieved from the corresponding Deployment/DaemonSet/ReplicaSet or StatefulSet.
func (f *Fetcher) getAnnotation(pod *corev1.Pod, key string) (string, error) {
	if value := pod.Annotations[key]; value != "" {
		return value, nil
	}

	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "DaemonSet":
			ds, err := f.k8s.Apps().V1().DaemonSets().Lister().DaemonSets(pod.Namespace).Get(owner.Name)
			if err != nil {
				return "", err
			}
			return ds.Annotations[key], nil

		case "StatefulSet":
			sts, err := f.k8s.Apps().V1().StatefulSets().Lister().StatefulSets(pod.Namespace).Get(owner.Name)
			if err != nil {
				return "", err
			}
			return sts.Annotations[key], nil

		case "ReplicaSet":
			rs, err := f.k8s.Apps().V1().ReplicaSets().Lister().ReplicaSets(pod.Namespace).Get(owner.Name)
			if err != nil {
				return "", err
			}

			if value := rs.Annotations[key]; value != "" {
				return value, nil
			}

			for _, owr := range rs.OwnerReferences {
				if owr.Kind != "Deployment" {
					continue
				}

				deploy, err := f.k8s.Apps().V1().Deployments().Lister().Deployments(pod.Namespace).Get(owr.Name)
				if err != nil {
					return "", err
				}
				return deploy.Annotations[key], nil
			}
		}
	}

	return "", nil
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
	var port string
	switch ctrl {
	case IngressControllerTypeTraefik:
		port = "8080"
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
	path = strings.TrimPrefix(path, "/")

	return fmt.Sprintf("http://%s/%s", net.JoinHostPort(pod.Status.PodIP, port), path)
}

func isSupportedIngressControllerType(value string) bool {
	switch value {
	case IngressControllerTypeHAProxyCommunity:
	case IngressControllerTypeTraefik:
	case IngressControllerTypeNone:
	default:
		return false
	}
	return true
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

func findEndpoints(svcs map[string]*Service, pod *corev1.Pod) []string {
	var endpoints []string
	for _, service := range svcs {
		if service.Namespace != pod.Namespace {
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
		for _, port := range service.ExternalPorts {
			endpoints = append(endpoints, fmt.Sprintf("%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, port))
		}
	}

	sort.Strings(endpoints)

	return endpoints
}

func findPublicEndpoints(svcs map[string]*Service, pod *corev1.Pod) []string {
	var endpoints []string

	knownPublicEndpoints := make(map[string]struct{})
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

		for _, ing := range service.status.LoadBalancer.Ingress {
			endpoint := ing.IP
			if endpoint == "" {
				endpoint = ing.Hostname
			}

			if _, exists := knownPublicEndpoints[endpoint]; exists {
				continue
			}

			knownPublicEndpoints[endpoint] = struct{}{}
			endpoints = append(endpoints, endpoint)
		}
	}

	sort.Strings(endpoints)

	return endpoints
}

func marshalToIngressClassNetworkingV1(ing *netv1beta1.IngressClass) (*netv1.IngressClass, error) {
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

func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
