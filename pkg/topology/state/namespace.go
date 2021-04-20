package state

import "k8s.io/apimachinery/pkg/labels"

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
