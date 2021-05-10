package state

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (f *Fetcher) getServices(apps map[string]*App) (map[string]*Service, map[string]string, error) {
	services, err := f.k8s.Core().V1().Services().Lister().List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}

	svcs := make(map[string]*Service)
	traefikNames := make(map[string]string)
	for _, service := range services {
		svcName := objectKey(service.Name, service.Namespace)
		svcs[svcName] = &Service{
			Name:      service.Name,
			Namespace: service.Namespace,
			Selector:  service.Spec.Selector,
			Apps:      selectApps(apps, service),
			Type:      service.Spec.Type,
			status:    service.Status,
		}

		for _, key := range traefikServiceNames(service) {
			traefikNames[key] = svcName
		}
	}

	return svcs, traefikNames, nil
}

func traefikServiceNames(svc *corev1.Service) []string {
	var result []string
	for _, port := range svc.Spec.Ports {
		result = append(result,
			fmt.Sprintf("%s-%s-%d", svc.Namespace, svc.Name, port.Port),
			fmt.Sprintf("%s-%s-%s", svc.Namespace, svc.Name, port.Name),
		)
	}
	return result
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
