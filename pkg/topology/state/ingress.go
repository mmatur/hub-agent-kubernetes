/*
Copyright (C) 2022 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package state

import (
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (f *Fetcher) getIngresses() (map[string]*Ingress, error) {
	ingresses, err := f.fetchIngresses()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Ingress)
	for _, ingress := range ingresses {
		ing := &Ingress{
			ResourceMeta: ResourceMeta{
				Kind:      "Ingress",
				Group:     netv1.GroupName,
				Name:      ingress.Name,
				Namespace: ingress.Namespace,
			},
			IngressMeta: IngressMeta{
				Annotations: sanitizeAnnotations(ingress.Annotations),
			},
			IngressClassName: ingress.Spec.IngressClassName,
			TLS:              ingress.Spec.TLS,
			DefaultBackend:   ingress.Spec.DefaultBackend,
			Rules:            ingress.Spec.Rules,
			Services:         getIngressServices(ingress),
		}

		result[ingressKey(ing.ResourceMeta)] = ing
	}

	return result, nil
}

func (f *Fetcher) fetchIngresses() ([]*netv1.Ingress, error) {
	ingresses, err := f.k8s.Networking().V1().Ingresses().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	v1beta1Ingresses, err := f.k8s.Networking().V1beta1().Ingresses().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, ingress := range v1beta1Ingresses {
		ing, err := toNetworkingV1(ingress)
		if err != nil {
			return nil, err
		}
		ingresses = append(ingresses, ing)
	}

	return ingresses, nil
}

func getIngressServices(ingress *netv1.Ingress) []string {
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
