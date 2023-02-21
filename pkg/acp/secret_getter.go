/*
Copyright (C) 2022-2023 Traefik Labs

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

package acp

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

// KubeSecretGetter allows getting Kubernetes secrets.
type KubeSecretGetter struct {
	secrets corev1lister.SecretLister
}

// NewKubeSecretValueGetter creates a KubeSecretGetter instance.
func NewKubeSecretValueGetter(secrets corev1lister.SecretLister) *KubeSecretGetter {
	return &KubeSecretGetter{secrets: secrets}
}

// GetValue returns the value of the given key in the given Kubernetes secret.
func (g KubeSecretGetter) GetValue(secret *corev1.SecretReference, key string) ([]byte, error) {
	s, err := g.secrets.Secrets(secret.Namespace).Get(secret.Name)
	if err != nil {
		return nil, fmt.Errorf("getting secret %q in namespace %q: %w", secret.Name, secret.Namespace, err)
	}

	value, ok := s.Data[key]
	if !ok {
		return nil, fmt.Errorf("no key %q in secret %q in namespace %q", key, secret.Name, secret.Namespace)
	}

	return value, nil
}

type emptySecretGetter struct{}

func (g emptySecretGetter) GetValue(*corev1.SecretReference, string) ([]byte, error) {
	return nil, nil
}
