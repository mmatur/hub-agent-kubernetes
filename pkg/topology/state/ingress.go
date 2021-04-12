package state

import (
	"github.com/hashicorp/go-version"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (f *Fetcher) getIngresses(clusterID string) (map[string]*Ingress, error) {
	ingresses, err := f.fetchIngresses()
	if err != nil {
		return nil, err
	}

	ingressClasses, err := f.fetchIngressClasses()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Ingress)
	for _, ingress := range ingresses {
		key := objectKey(ingress.Name, ingress.Namespace)

		result[key] = &Ingress{
			Name:           ingress.Name,
			Namespace:      ingress.Namespace,
			ClusterID:      clusterID,
			Annotations:    sanitizeAnnotations(ingress.Annotations),
			TLS:            ingress.Spec.TLS,
			DefaultService: ingress.Spec.DefaultBackend,
			Rules:          ingress.Spec.Rules,
			Controller:     getControllerName(ingress, ingressClasses),
			Services:       getServices(ingress),
		}
	}

	return result, nil
}

func (f *Fetcher) fetchIngresses() ([]*netv1.Ingress, error) {
	var result []*netv1.Ingress

	if f.serverVersion.GreaterThanOrEqual(version.Must(version.NewVersion("1.19"))) {
		var err error
		result, err = f.k8s.Networking().V1().Ingresses().Lister().List(labels.Everything())
		if err != nil {
			return nil, err
		}

		return result, nil
	}

	v1beta1Ingresses, err := f.k8s.Networking().V1beta1().Ingresses().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, ingress := range v1beta1Ingresses {
		networking, err := toNetworkingV1(ingress)
		if err != nil {
			return nil, err
		}
		result = append(result, networking)
	}

	return result, nil
}

func sanitizeAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}

	result := make(map[string]string)
	for name, value := range annotations {
		if name == "kubectl.kubernetes.io/last-applied-configuration" {
			continue
		}

		result[name] = value
	}

	return result
}

func getServices(ingress *netv1.Ingress) []string {
	var result []string

	knownServices := make(map[string]struct{})

	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		key := objectKey(ingress.Spec.DefaultBackend.Service.Name, ingress.Namespace)
		knownServices[key] = struct{}{}
		result = append(result, key)
	}

	for _, r := range ingress.Spec.Rules {
		for _, p := range r.HTTP.Paths {
			if p.Backend.Service == nil {
				continue
			}

			key := objectKey(p.Backend.Service.Name, ingress.Namespace)
			if _, exists := knownServices[key]; exists {
				continue
			}

			knownServices[key] = struct{}{}
			result = append(result, key)
		}
	}

	return result
}

func getControllerName(ingress *netv1.Ingress, ingressClasses []*netv1.IngressClass) string {
	// Look for ingressClassName in Ingress spec.
	var ingressClassName string
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
		ingressClassName = *ingress.Spec.IngressClassName
	}

	// Look for ingressClassName in annotations if it was not found in the Ingress spec.
	if ingressClassName == "" {
		// TODO: For now we don't support custom ingress class names so this could break.
		return ingress.Annotations["kubernetes.io/ingress.class"]
	}

	// Find the matching ingress class.
	var ingressClass *netv1.IngressClass
	for _, ic := range ingressClasses {
		if ic.Name == ingressClassName {
			ingressClass = ic
			break
		}
	}

	if ingressClass == nil {
		return ingressClassName
	}

	switch ingressClass.Spec.Controller {
	case ControllerTypeNginxCommunity:
		return IngressControllerTypeNginxCommunity

	case ControllerTypeNginxOfficial:
		return IngressControllerTypeNginxOfficial

	case ControllerTypeTraefik:
		return IngressControllerTypeTraefik

	case ControllerTypeHAProxyCommunity:
		return IngressControllerTypeHAProxyCommunity

	default:
		return ingressClass.Spec.Controller
	}
}

func toNetworkingV1(ing *netv1beta1.Ingress) (*netv1.Ingress, error) {
	netv1Ingress, err := marshalToIngressNetworkingV1(ing)
	if err != nil {
		return nil, err
	}

	if ing.Spec.Backend != nil {
		var port netv1.ServiceBackendPort
		switch ing.Spec.Backend.ServicePort.Type {
		case intstr.Int:
			port.Number = ing.Spec.Backend.ServicePort.IntVal
		case intstr.String:
			port.Name = ing.Spec.Backend.ServicePort.StrVal
		}

		netv1Ingress.Spec.DefaultBackend = &netv1.IngressBackend{
			Service: &netv1.IngressServiceBackend{
				Name: ing.Spec.Backend.ServiceName,
				Port: port,
			},
			Resource: ing.Spec.Backend.Resource,
		}
	}

	for _, rule := range netv1Ingress.Spec.Rules {
		for index, path := range rule.HTTP.Paths {
			var backend *netv1beta1.IngressBackend
			if backend = findBackend(ing, rule.Host, path.Path); backend == nil {
				continue
			}

			var port netv1.ServiceBackendPort
			switch backend.ServicePort.Type {
			case intstr.Int:
				port.Number = backend.ServicePort.IntVal
			case intstr.String:
				port.Name = backend.ServicePort.StrVal
			}

			rule.HTTP.Paths[index].Backend = netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: backend.ServiceName,
					Port: port,
				},
			}
		}
	}

	return netv1Ingress, nil
}

func findBackend(ingress *netv1beta1.Ingress, host, path string) *netv1beta1.IngressBackend {
	for _, rule := range ingress.Spec.Rules {
		if rule.Host != host {
			continue
		}

		for _, rulePath := range rule.HTTP.Paths {
			if rulePath.Path == path {
				return &rulePath.Backend
			}
		}
	}

	return nil
}

func marshalToIngressNetworkingV1(ing *netv1beta1.Ingress) (*netv1.Ingress, error) {
	data, err := ing.Marshal()
	if err != nil {
		return nil, err
	}

	ni := &netv1.Ingress{}
	if err := ni.Unmarshal(data); err != nil {
		return nil, err
	}

	return ni, nil
}
