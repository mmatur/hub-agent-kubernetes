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

package api

import (
	"encoding/base64"
	"fmt"
	"time"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Access defines who can access APIs.
type Access struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`

	Groups                []string              `json:"groups"`
	APISelector           *metav1.LabelSelector `json:"apiSelector,omitempty"`
	APICollectionSelector *metav1.LabelSelector `json:"apiCollectionSelector,omitempty"`

	Version string `json:"version"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Resource builds the v1alpha1 APIAccess resource.
func (a *Access) Resource() (*hubv1alpha1.APIAccess, error) {
	access := &hubv1alpha1.APIAccess{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APIAccess",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   a.Name,
			Labels: a.Labels,
		},
		Spec: hubv1alpha1.APIAccessSpec{
			Groups:                a.Groups,
			APISelector:           a.APISelector,
			APICollectionSelector: a.APICollectionSelector,
		},
		Status: hubv1alpha1.APIAccessStatus{
			Version:  a.Version,
			SyncedAt: metav1.Now(),
		},
	}

	h, err := HashAccess(access)
	if err != nil {
		return nil, fmt.Errorf("compute APIAccess hash: %w", err)
	}

	access.Status.Hash = h

	return access, nil
}

type accessHash struct {
	Groups                []string          `json:"groups"`
	APISelector           string            `json:"apiSelector"`
	APICollectionSelector string            `json:"apiCollectionSelector"`
	Labels                sortedMap[string] `json:"labels"`
}

// HashAccess generates the hash of the APIAccess.
func HashAccess(a *hubv1alpha1.APIAccess) (string, error) {
	ah := accessHash{
		Groups: a.Spec.Groups,
		Labels: newSortedMap(a.Labels),
	}
	if a.Spec.APISelector != nil {
		ah.APISelector = a.Spec.APISelector.String()
	}
	if a.Spec.APICollectionSelector != nil {
		ah.APICollectionSelector = a.Spec.APICollectionSelector.String()
	}

	hash, err := sum(ah)
	if err != nil {
		return "", fmt.Errorf("sum object: %w", err)
	}

	return base64.StdEncoding.EncodeToString(hash), nil
}
