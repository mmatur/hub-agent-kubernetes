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

// API is an API exposed within a portal.
type API struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels,omitempty"`
	PathPrefix string            `json:"pathPrefix"`
	Service    Service           `json:"service"`

	Version string `json:"version"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Service is a Kubernetes Service.
type Service struct {
	Name string `json:"name" bson:"name"`
	Port int    `json:"port" bson:"port"`

	OpenAPISpec OpenAPISpec `json:"openApiSpec,omitempty" bson:"openApiSpec,omitempty"`
}

// OpenAPISpec is an OpenAPISpec. It can either be fetched from a URL, or Path/Port from the service
// or directly in the Schema field.
type OpenAPISpec struct {
	URL string `json:"url,omitempty" bson:"url,omitempty"`

	Path string `json:"path,omitempty" bson:"path,omitempty"`
	Port int    `json:"port,omitempty" bson:"port,omitempty"`
}

// Resource builds the v1alpha1 API resource.
func (a *API) Resource() (*hubv1alpha1.API, error) {
	api := &hubv1alpha1.API{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "API",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Name,
			Namespace: a.Namespace,
			Labels:    a.Labels,
		},
		Spec: hubv1alpha1.APISpec{
			PathPrefix: a.PathPrefix,
			Service: hubv1alpha1.APIService{
				Name: a.Service.Name,
				Port: hubv1alpha1.APIServiceBackendPort{
					Number: int32(a.Service.Port),
				},
				OpenAPISpec: hubv1alpha1.OpenAPISpec{
					URL:  a.Service.OpenAPISpec.URL,
					Path: a.Service.OpenAPISpec.Path,
				},
			},
		},
		Status: hubv1alpha1.APIStatus{
			Version:  a.Version,
			SyncedAt: metav1.Now(),
		},
	}

	if a.Service.OpenAPISpec.Port != 0 {
		api.Spec.Service.OpenAPISpec.Port = &hubv1alpha1.APIServiceBackendPort{
			Number: int32(a.Service.OpenAPISpec.Port),
		}
	}

	apiHash, err := HashAPI(api)
	if err != nil {
		return nil, fmt.Errorf("compute API hash: %w", err)
	}

	api.Status.Hash = apiHash

	return api, nil
}

type apiHash struct {
	PathPrefix string                 `json:"pathPrefix,omitempty"`
	Service    hubv1alpha1.APIService `json:"service"`
	Labels     sortedMap[string]      `json:"labels,omitempty"`
}

// HashAPI generates the hash of the API.
func HashAPI(a *hubv1alpha1.API) (string, error) {
	ah := apiHash{
		PathPrefix: a.Spec.PathPrefix,
		Service:    a.Spec.Service,
		Labels:     newSortedMap(a.Labels),
	}

	hash, err := sum(ah)
	if err != nil {
		return "", fmt.Errorf("sum object: %w", err)
	}

	return base64.StdEncoding.EncodeToString(hash), nil
}
