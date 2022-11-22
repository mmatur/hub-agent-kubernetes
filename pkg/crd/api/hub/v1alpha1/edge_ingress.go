/*
Copyright (C) 2022 Traefik Labs

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
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EdgeIngress defines an edge ingress.
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.spec.service.name`
// +kubebuilder:printcolumn:name="Port",type=string,JSONPath=`.spec.service.port`
// +kubebuilder:printcolumn:name="ACP",type=string,JSONPath=`.spec.acp.name`,priority=1
// +kubebuilder:printcolumn:name="URLs",type=string,JSONPath=`.status.urls`
// +kubebuilder:printcolumn:name="Connection",type=string,JSONPath=`.status.connection`
type EdgeIngress struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The desired behavior of this edge ingress.
	Spec EdgeIngressSpec `json:"spec,omitempty"`

	// The current status of this edge ingress.
	// +optional
	Status EdgeIngressStatus `json:"status,omitempty"`
}

// EdgeIngressSpec configures an edgeIngress policy.
type EdgeIngressSpec struct {
	Service EdgeIngressService `json:"service"`
	ACP     *EdgeIngressACP    `json:"acp,omitempty"`
	// CustomDomains are the custom domains for accessing the exposed service.
	CustomDomains []string `json:"customDomains,omitempty"`
}

// Hash generates the hash of the spec.
func (in *EdgeIngressSpec) Hash() (string, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("encode ACP: %w", err)
	}

	hash := sha1.New() //nolint:gosec // Used for content diffing, no impact on security
	hash.Write(b)

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}

// EdgeIngressService configures the service to exposed on the edge.
type EdgeIngressService struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// EdgeIngressACP configures the ACP to use on the Ingress.
type EdgeIngressACP struct {
	Name string `json:"name"`
}

// EdgeIngressConnectionStatus is the status of the underlying connection to the edge.
type EdgeIngressConnectionStatus string

// Connection statuses.
const (
	EdgeIngressConnectionDown EdgeIngressConnectionStatus = "DOWN"
	EdgeIngressConnectionUp   EdgeIngressConnectionStatus = "UP"
)

// EdgeIngressStatus is the status of the EdgeIngress.
type EdgeIngressStatus struct {
	Version  string      `json:"version,omitempty"`
	SyncedAt metav1.Time `json:"syncedAt,omitempty"`

	// Domain is the Domain for accessing the exposed service.
	Domain string `json:"domain,omitempty"`

	// CustomDomains are the custom domains for accessing the exposed service.
	CustomDomains []string `json:"customDomains,omitempty"`

	// URLs is the list of coma separated URL for accessing the exposed service.
	URLs string `json:"urls,omitempty"`

	// Connection is the status of the underlying connection to the edge.
	Connection EdgeIngressConnectionStatus `json:"connection,omitempty"`

	// SpecHash is a hash representing the EdgeIngressSpec
	SpecHash string `json:"specHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EdgeIngressList defines a list of edge ingress.
type EdgeIngressList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EdgeIngress `json:"items"`
}
