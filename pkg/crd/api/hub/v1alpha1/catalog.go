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
	"crypto/sha1" //nolint:gosec // Used for content diffing, no impact on security
	"encoding/base64"
	"encoding/json"
	"fmt"

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
	// CustomDomains are the custom domains under which the API will be exposed.
	CustomDomains []string `json:"customDomains,omitempty"`
	// Services are the list of Services available in the Catalog.
	Services []CatalogService `json:"services,omitempty"`
}

// Hash generates the hash of the spec.
func (in *CatalogSpec) Hash() (string, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("encode Catalog: %w", err)
	}

	hash := sha1.New() //nolint:gosec // Used for content diffing, no impact on security
	hash.Write(b)

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
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

	// Domains are the domains of the Catalog API.
	Domains []string `json:"domains"`

	// DevPortalDomain is the domain for accessing the dev portal.
	DevPortalDomain string `json:"devPortalDomain"`

	// SpecHash is a hash representing the CatalogSpec
	SpecHash string `json:"specHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CatalogList defines a list of Catalog.
type CatalogList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Catalog `json:"items"`
}
