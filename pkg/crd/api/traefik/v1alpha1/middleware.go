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
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Middleware is a specification for a Middleware resource.
type Middleware struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec MiddlewareSpec `json:"spec"`
}

// +k8s:deepcopy-gen=true

// MiddlewareSpec holds the Middleware configuration.
type MiddlewareSpec struct {
	ForwardAuth      *ForwardAuth      `json:"forwardAuth,omitempty"`
	StripPrefix      *StripPrefix      `json:"stripPrefix,omitempty"`
	StripPrefixRegex *StripPrefixRegex `json:"stripPrefixRegex,omitempty"`
	AddPrefix        *AddPrefix        `json:"addPrefix,omitempty"`
}

// +k8s:deepcopy-gen=true

// AddPrefix holds the AddPrefix configuration.
type AddPrefix struct {
	Prefix string `json:"prefix,omitempty" toml:"prefix,omitempty" yaml:"prefix,omitempty" export:"true"`
}

// +k8s:deepcopy-gen=true

// StripPrefix holds the StripPrefix configuration.
type StripPrefix struct {
	Prefixes   []string `json:"prefixes,omitempty" toml:"prefixes,omitempty" yaml:"prefixes,omitempty" export:"true"`
	ForceSlash bool     `json:"forceSlash,omitempty" toml:"forceSlash,omitempty" yaml:"forceSlash,omitempty" export:"true"` // Deprecated
}

// +k8s:deepcopy-gen=true

// StripPrefixRegex holds the StripPrefixRegex configuration.
type StripPrefixRegex struct {
	Regex []string `json:"regex,omitempty" toml:"regex,omitempty" yaml:"regex,omitempty" export:"true"`
}

// +k8s:deepcopy-gen=true

// ForwardAuth holds the http forward authentication configuration.
type ForwardAuth struct {
	Address                  string     `json:"address,omitempty"`
	TrustForwardHeader       bool       `json:"trustForwardHeader,omitempty"`
	AuthResponseHeaders      []string   `json:"authResponseHeaders,omitempty"`
	AuthResponseHeadersRegex string     `json:"authResponseHeadersRegex,omitempty"`
	AuthRequestHeaders       []string   `json:"authRequestHeaders,omitempty"`
	TLS                      *ClientTLS `json:"tls,omitempty"`
}

// ClientTLS holds TLS specific configurations as client.
type ClientTLS struct {
	CASecret           string `json:"caSecret,omitempty"`
	CAOptional         bool   `json:"caOptional,omitempty"`
	CertSecret         string `json:"certSecret,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MiddlewareList is a list of Middleware resources.
type MiddlewareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Middleware `json:"items"`
}
