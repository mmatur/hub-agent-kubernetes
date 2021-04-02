package state

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (f *Fetcher) getServices(apps map[string]*App) (map[string]*Service, error) {
	services, err := f.k8s.Core().V1().Services().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Service)
	for _, service := range services {
		result[objectKey(service.Name, service.Namespace)] = &Service{
			Name:      service.Name,
			Namespace: service.Namespace,
			Selector:  service.Spec.Selector,
			Apps:      selectApps(apps, service),
			Type:      service.Spec.Type,
			status:    service.Status,
		}
	}

	return result, nil
}

func selectApps(apps map[string]*App, service *corev1.Service) []string {
	if service.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}

	var result []string
	for key, app := range apps {
		if app.Namespace != service.Namespace {
			continue
		}

		var match bool
		for k, v := range service.Spec.Selector {
			if app.podLabels[k] != v {
				match = false
				break
			}
			match = true
		}

		if match {
			result = append(result, key)
		}
	}

	sort.Strings(result)

	return result
}
