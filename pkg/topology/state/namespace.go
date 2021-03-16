package state

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (f *Fetcher) getNamespaces() ([]string, error) {
	ns, err := f.k8s.Core().V1().Namespaces().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var result []string
	for _, namespace := range ns {
		result = append(result, namespace.Name)
	}

	return result, nil
}

func (f *Fetcher) getClusterID() (string, error) {
	ns, err := f.k8s.Core().V1().Namespaces().Lister().List(labels.Everything())
	if err != nil {
		return "", err
	}

	for _, namespace := range ns {
		if namespace.Name == metav1.NamespaceSystem {
			return string(namespace.UID), nil
		}
	}

	return "", errors.New("could not find kube-system namespace UID")
}
