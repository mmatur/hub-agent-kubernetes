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
	"crypto/sha1" //nolint:gosec // Used for content diffing, no impact on security
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Collection is a collection of APIs exposed within an APIPortal.
type Collection struct {
	Name        string               `json:"name"`
	PathPrefix  string               `json:"pathPrefix,omitempty"`
	APISelector metav1.LabelSelector `json:"apiSelector"`

	Version string `json:"version"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Resource builds the v1alpha1 Collection resource.
func (c *Collection) Resource() (*hubv1alpha1.APICollection, error) {
	collection := &hubv1alpha1.APICollection{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APICollection",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: c.Name,
		},
		Spec: hubv1alpha1.APICollectionSpec{
			PathPrefix:  c.PathPrefix,
			APISelector: c.APISelector,
		},
		Status: hubv1alpha1.APICollectionStatus{
			Version:  c.Version,
			SyncedAt: metav1.Now(),
		},
	}

	collectionHash, err := hashCollection(collection)
	if err != nil {
		return nil, fmt.Errorf("compute APICollection hash: %w", err)
	}

	collection.Status.Hash = collectionHash

	return collection, nil
}

type collectionHash struct {
	PathPrefix  string               `json:"pathPrefix,omitempty"`
	APISelector metav1.LabelSelector `json:"apiSelector"`
}

func hashCollection(c *hubv1alpha1.APICollection) (string, error) {
	ch := collectionHash{
		PathPrefix:  c.Spec.PathPrefix,
		APISelector: c.Spec.APISelector,
	}

	b, err := json.Marshal(ch)
	if err != nil {
		return "", fmt.Errorf("encode APICollection: %w", err)
	}

	hash := sha1.New() //nolint:gosec // Used for content diffing, no impact on security
	hash.Write(b)

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}
