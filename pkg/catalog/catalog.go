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

package catalog

import (
	"fmt"
	"strings"
	"time"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Catalog is a catalog of services exposed through a unified API.
type Catalog struct {
	WorkspaceID string `json:"workspaceId"`
	ClusterID   string `json:"clusterId"`
	Name        string `json:"name"`

	Version string `json:"version"`

	Domain        string    `json:"domain"`
	CustomDomains []string  `json:"customDomains"`
	Services      []Service `json:"services,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Service is a service within a catalog.
type Service = hubv1alpha1.CatalogService

// Resource builds the v1alpha1 Catalog resource.
func (c *Catalog) Resource(oasRegistry OASRegistry) (*hubv1alpha1.Catalog, error) {
	var serviceStatuses []hubv1alpha1.CatalogServiceStatus
	for _, svc := range c.Services {
		svc := svc

		openAPISpecURL := svc.OpenAPISpecURL
		if openAPISpecURL == "" {
			openAPISpecURL = oasRegistry.GetURL(svc.Name, svc.Namespace)
		}

		serviceStatuses = append(serviceStatuses, hubv1alpha1.CatalogServiceStatus{
			Name:           svc.Name,
			Namespace:      svc.Namespace,
			OpenAPISpecURL: openAPISpecURL,
		})
	}

	spec := hubv1alpha1.CatalogSpec{
		CustomDomains: c.CustomDomains,
		Services:      c.Services,
	}

	specHash, err := spec.Hash()
	if err != nil {
		return nil, fmt.Errorf("compute spec hash: %w", err)
	}

	urls := []string{"https://" + c.Domain}
	for _, customDomain := range c.CustomDomains {
		urls = append(urls, "https://"+customDomain)
	}

	return &hubv1alpha1.Catalog{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "Catalog",
		},
		ObjectMeta: metav1.ObjectMeta{Name: c.Name},
		Spec:       spec,
		Status: hubv1alpha1.CatalogStatus{
			Version:  c.Version,
			SyncedAt: metav1.Now(),
			Domain:   c.Domain,
			URLs:     strings.Join(urls, ","),
			SpecHash: specHash,
			Services: serviceStatuses,
		},
	}, nil
}
