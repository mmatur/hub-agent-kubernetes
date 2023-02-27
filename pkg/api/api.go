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

// API is an API exposed within a portal.
type API struct {
	Name       string                 `json:"name"`
	Namespace  string                 `json:"namespace"`
	PathPrefix string                 `json:"pathPrefix"`
	Service    hubv1alpha1.APIService `json:"service"`

	Version string `json:"version"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
		},
		Spec: hubv1alpha1.APISpec{
			PathPrefix: a.PathPrefix,
			Service:    a.Service,
		},
		Status: hubv1alpha1.APIStatus{
			Version:  a.Version,
			SyncedAt: metav1.Now(),
		},
	}

	apiHash, err := hashAPI(api)
	if err != nil {
		return nil, fmt.Errorf("compute api hash: %w", err)
	}

	api.Status.Hash = apiHash

	return api, nil
}

type apiHash struct {
	PathPrefix string                 `json:"pathPrefix,omitempty"`
	Service    hubv1alpha1.APIService `json:"service"`
}

func hashAPI(a *hubv1alpha1.API) (string, error) {
	ah := apiHash{
		PathPrefix: a.Spec.PathPrefix,
		Service:    a.Spec.Service,
	}

	b, err := json.Marshal(ah)
	if err != nil {
		return "", fmt.Errorf("encode api: %w", err)
	}

	hash := sha1.New() //nolint:gosec // Used for content diffing, no impact on security
	hash.Write(b)

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}
