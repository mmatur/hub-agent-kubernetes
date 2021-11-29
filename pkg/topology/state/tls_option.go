package state

import "k8s.io/apimachinery/pkg/labels"

func (f *Fetcher) getTLSOptions() (map[string]*TLSOptions, error) {
	tlsOptions, err := f.traefik.Traefik().V1alpha1().TLSOptions().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*TLSOptions)
	for _, tlsOption := range tlsOptions {
		result[objectKey(tlsOption.Name, tlsOption.Namespace)] = &TLSOptions{
			Name:                     tlsOption.Name,
			Namespace:                tlsOption.Namespace,
			MinVersion:               tlsOption.Spec.MinVersion,
			MaxVersion:               tlsOption.Spec.MaxVersion,
			CipherSuites:             tlsOption.Spec.CipherSuites,
			CurvePreferences:         tlsOption.Spec.CurvePreferences,
			ClientAuth:               tlsOption.Spec.ClientAuth,
			SniStrict:                tlsOption.Spec.SniStrict,
			PreferServerCipherSuites: tlsOption.Spec.PreferServerCipherSuites,
		}
	}

	return result, nil
}
