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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Catalog defines a catalog.
// +kubebuilder:printcolumn:name="URLs",type=string,JSONPath=`.status.urls`
// +kubebuilder:resource:scope=Cluster
type Catalog struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The desired behavior of this catalog.
	Spec CatalogSpec `json:"spec,omitempty"`

	// The current status of this catalog.
	// +optional
	Status CatalogStatus `json:"status,omitempty"`
}

// CatalogSpec configures a Catalog.
type CatalogSpec struct {
	// +optional
	Description string `json:"description,omitempty"`
	// CustomDomains are the custom domains under which the API will be exposed.
	// +optional
	CustomDomains []string `json:"customDomains,omitempty"`
	// Services are the list of Services available in the Catalog.
	Services []CatalogService `json:"services,omitempty"`
}

// CatalogService defines how a Kubernetes Service is exposed within a Catalog.
type CatalogService struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Port       int    `json:"port"`
	PathPrefix string `json:"pathPrefix"`
	// +optional
	OpenAPISpecURL string `json:"openApiSpecUrl,omitempty"`
}

// CatalogStatus is the status of a Catalog.
type CatalogStatus struct {
	Version  string      `json:"version,omitempty"`
	SyncedAt metav1.Time `json:"syncedAt,omitempty"`

	// URLs are the URLs for accessing the Catalog API.
	URLs string `json:"urls"`

	// Domain is the hub generated domain of the Catalog API.
	Domain string `json:"domain"`

	// CustomDomains are the custom domains for accessing the exposed service.
	CustomDomains []string `json:"customDomains,omitempty"`

	// DevPortalDomain is the domain for accessing the dev portal.
	// +optional
	DevPortalDomain string `json:"devPortalDomain"`

	// Hash is a hash representing the Catalog.
	Hash string `json:"hash,omitempty"`

	Services []CatalogServiceStatus `json:"services,omitempty"`
}

// CatalogServiceStatus is the status of a Service within a Catalog.
type CatalogServiceStatus struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	OpenAPISpecURL string `json:"openApiSpecUrl,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CatalogList defines a list of Catalog.
type CatalogList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Catalog `json:"items"`
}
