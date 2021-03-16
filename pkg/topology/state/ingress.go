package state

import (
	"strings"

	"github.com/hashicorp/go-version"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (f *Fetcher) getIngresses(controllers map[string]*IngressController) (map[string]*Ingress, error) {
	ingresses, err := f.fetchIngresses()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Ingress)
	for _, ingress := range ingresses {
		var ingressTLS []IngressTLS
		for _, tls := range ingress.Spec.TLS {
			ingressTLS = append(ingressTLS, IngressTLS{
				Hosts:      tls.Hosts,
				SecretName: tls.SecretName,
			})
		}

		key := objectKey(ingress.Name, ingress.Namespace)
		result[key] = &Ingress{
			Name:               ingress.Name,
			Namespace:          ingress.Namespace,
			Annotations:        ingress.Annotations,
			TLS:                ingressTLS,
			Status:             ingress.Status.LoadBalancer,
			CertManagerEnabled: usesCertManager(ingress),
			Controller:         getControllerName(ingress, controllers),
		}

		// Set services without duplicates.
		knownServices := make(map[string]struct{})
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				if _, exists := knownServices[path.Backend.Service.Name]; exists {
					continue
				}

				knownServices[path.Backend.Service.Name] = struct{}{}

				result[key].Services = append(result[key].Services, path.Backend.Service.Name)
			}
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

func usesCertManager(ingress *netv1.Ingress) bool {
	for annotation := range ingress.Annotations {
		if strings.Contains(annotation, "cert-manager.io") || strings.Contains(annotation, "certmanager.k8s.io") {
			return true
		}
	}

	return false
}

func getControllerName(ingress *netv1.Ingress, controllers map[string]*IngressController) string {
	// Look for ingressClassName in Ingress spec.
	var ingressClassName string
	if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName != "" {
		ingressClassName = *ingress.Spec.IngressClassName
	}

	// Look for ingressClassName in annotations if it was not found in the Ingress spec.
	if ingressClassName == "" {
		igc, ok := ingress.Annotations["kubernetes.io/ingress.class"]
		if ok {
			ingressClassName = igc
		}
	}

	// Look for the controller that handles the IngressClass.
	for _, ctrl := range controllers {
		for _, ingressClass := range ctrl.IngressClasses {
			if ingressClassName == ingressClass {
				return ctrl.Name
			}
		}
	}

	// TODO: Handle cases where no ingress classes are given and we could guess which ingress controller handles this ingress.
	return ""
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
