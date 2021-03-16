package state

import (
	"net"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

func (f *Fetcher) getServices() (map[string]*Service, error) {
	services, err := f.k8s.Core().V1().Services().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Service)
	for _, service := range services {
		endpoint, err := f.k8s.Core().V1().Endpoints().Lister().Endpoints(service.Namespace).Get(service.Name)
		if err != nil && !kerror.IsNotFound(err) {
			return nil, err
		}

		result[objectKey(service.Name, service.Namespace)] = &Service{
			Name:      service.Name,
			Namespace: service.Namespace,
			Status:    service.Status,
			Selector:  service.Spec.Selector,
			addresses: getAddressesFromEndpoint(endpoint),
		}
	}

	return result, nil
}

func getAddressesFromEndpoint(endpoint *corev1.Endpoints) []string {
	if endpoint == nil {
		return nil
	}

	var addrs []string

	for _, subset := range endpoint.Subsets {
		for _, address := range subset.Addresses {
			for _, port := range subset.Ports {
				p := strconv.FormatInt(int64(port.Port), 10)
				addrs = append(addrs, net.JoinHostPort(address.IP, p))
			}
		}
	}

	sort.Strings(addrs)

	return addrs
}
