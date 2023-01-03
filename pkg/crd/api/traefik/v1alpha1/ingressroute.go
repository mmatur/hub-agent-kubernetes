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
	"k8s.io/apimachinery/pkg/util/intstr"
)

// IngressRouteSpec is a specification for a IngressRouteSpec resource.
type IngressRouteSpec struct {
	Routes      []Route  `json:"routes"`
	EntryPoints []string `json:"entryPoints,omitempty"`
	TLS         *TLS     `json:"tls,omitempty"`
}

// Route contains the set of routes.
type Route struct {
	Match string `json:"match"`
	// +kubebuilder:validation:Enum=Rule
	Kind        string          `json:"kind"`
	Priority    int             `json:"priority,omitempty"`
	Services    []Service       `json:"services,omitempty"`
	Middlewares []MiddlewareRef `json:"middlewares,omitempty"`
}

// TLS contains the TLS certificates configuration of the routes.
// To enable Let's Encrypt, use an empty TLS struct,
// e.g. in YAML:
//
//	tls: {} # inline format
//
//	tls:
//	  secretName: # block format
type TLS struct {
	// SecretName is the name of the referenced Kubernetes Secret to specify the
	// certificate details.
	SecretName string `json:"secretName,omitempty"`
	// Options is a reference to a TLSOption, that specifies the parameters of the TLS connection.
	Options *TLSOptionRef `json:"options,omitempty"`
	// Store is a reference to a TLSStore, that specifies the parameters of the TLS store.
	Store        *TLSStoreRef `json:"store,omitempty"`
	CertResolver string       `json:"certResolver,omitempty"`
	Domains      []Domain     `json:"domains,omitempty"`
}

// TLSOptionRef is a ref to the TLSOption resources.
type TLSOptionRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// TLSStoreRef is a ref to the TLSStore resource.
type TLSStoreRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// LoadBalancerSpec can reference either a Kubernetes Service object (a load-balancer of servers),
// or a TraefikService object (a traefik load-balancer of services).
type LoadBalancerSpec struct {
	// Name is a reference to a Kubernetes Service object (for a load-balancer of servers),
	// or to a TraefikService object (service load-balancer, mirroring, etc).
	// The differentiation between the two is specified in the Kind field.
	Name string `json:"name"`
	// +kubebuilder:validation:Enum=Service;TraefikService
	Kind      string  `json:"kind,omitempty"`
	Namespace string  `json:"namespace,omitempty"`
	Sticky    *Sticky `json:"sticky,omitempty"`

	// Port and all the fields below are related to a servers load-balancer,
	// and therefore should only be specified when Name references a Kubernetes Service.

	Port               intstr.IntOrString  `json:"port,omitempty"`
	Scheme             string              `json:"scheme,omitempty"`
	Strategy           string              `json:"strategy,omitempty"`
	PassHostHeader     *bool               `json:"passHostHeader,omitempty"`
	ResponseForwarding *ResponseForwarding `json:"responseForwarding,omitempty"`
	ServersTransport   string              `json:"serversTransport,omitempty"`

	// Weight should only be specified when Name references a TraefikService object
	// (and to be precise, one that embeds a Weighted Round Robin).
	Weight *int `json:"weight,omitempty"`
}

// Service defines an upstream to proxy traffic.
type Service struct {
	LoadBalancerSpec `json:",inline"`
}

// MiddlewareRef is a ref to the Middleware resources.
type MiddlewareRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion

// IngressRoute is an Ingress CRD specification.
type IngressRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec IngressRouteSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressRouteList is a list of IngressRoutes.
type IngressRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []IngressRoute `json:"items"`
}

// +k8s:deepcopy-gen=true

// Cookie holds the sticky configuration based on cookie.
type Cookie struct {
	Name     string `json:"name,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

// +k8s:deepcopy-gen=true

// Domain holds a domain name with SANs.
type Domain struct {
	Main string   `description:"Default subject name." json:"main,omitempty"`
	SANs []string `description:"Subject alternative names." json:"sans,omitempty"`
}

// +k8s:deepcopy-gen=true

// ResponseForwarding holds configuration for the forward of the response.
type ResponseForwarding struct {
	FlushInterval string `json:"flushInterval,omitempty"`
}
