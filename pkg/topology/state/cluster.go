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

package state

import (
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
)

// Cluster describes a Cluster.
type Cluster struct {
	Ingresses             map[string]*Ingress             `json:"ingresses"`
	IngressRoutes         map[string]*IngressRoute        `json:"ingressRoutes"`
	Services              map[string]*Service             `json:"services"`
	AccessControlPolicies map[string]*AccessControlPolicy `json:"accessControlPolicies"`
	EdgeIngresses         map[string]*EdgeIngress         `json:"edgeIngresses"`
}

// ResourceMeta represents the metadata which identify a Kubernetes resource.
type ResourceMeta struct {
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// Service describes a Service.
type Service struct {
	Name                string               `json:"name"`
	Namespace           string               `json:"namespace"`
	Type                corev1.ServiceType   `json:"type"`
	Annotations         map[string]string    `json:"annotations,omitempty"`
	ExternalIPs         []string             `json:"externalIPs,omitempty"`
	ExternalPorts       []int                `json:"externalPorts,omitempty"`
	OpenAPISpecLocation *OpenAPISpecLocation `json:"openApiSpecLocation,omitempty"`
}

// OpenAPISpecLocation describes the location of an OpenAPI specification.
type OpenAPISpecLocation struct {
	Path string `json:"path"`
	Port int    `json:"port"`
}

// IngressMeta represents the common Ingress metadata properties.
type IngressMeta struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Ingress describes an Kubernetes Ingress.
type Ingress struct {
	ResourceMeta
	IngressMeta

	IngressClassName *string               `json:"ingressClassName,omitempty"`
	TLS              []netv1.IngressTLS    `json:"tls,omitempty"`
	Rules            []netv1.IngressRule   `json:"rules,omitempty"`
	DefaultBackend   *netv1.IngressBackend `json:"defaultBackend,omitempty"`
	Services         []string              `json:"services,omitempty"`
}

// IngressRoute describes a Traefik IngressRoute.
type IngressRoute struct {
	ResourceMeta
	IngressMeta

	TLS      *IngressRouteTLS `json:"tls,omitempty"`
	Routes   []Route          `json:"routes,omitempty"`
	Services []string         `json:"services,omitempty"`
}

// IngressRouteTLS represents a simplified Traefik IngressRoute TLS configuration.
type IngressRouteTLS struct {
	Domains    []traefikv1alpha1.Domain `json:"domains,omitempty"`
	SecretName string                   `json:"secretName,omitempty"`
	Options    *TLSOptionRef            `json:"options,omitempty"`
}

// TLSOptionRef references TLSOptions.
type TLSOptionRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// Route represents a Traefik IngressRoute route.
type Route struct {
	Match    string         `json:"match"`
	Services []RouteService `json:"services,omitempty"`
}

// RouteService represents a Kubernetes service targeted by a Traefik IngressRoute route.
type RouteService struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	PortName   string `json:"portName,omitempty"`
	PortNumber int32  `json:"portNumber,omitempty"`
}

// AccessControlPolicy describes an Access Control Policy configured within a cluster.
type AccessControlPolicy struct {
	Name       string                         `json:"name"`
	Method     string                         `json:"method"`
	JWT        *AccessControlPolicyJWT        `json:"jwt,omitempty"`
	APIKey     *AccessControlPolicyAPIKey     `json:"apiKey,omitempty"`
	BasicAuth  *AccessControlPolicyBasicAuth  `json:"basicAuth,omitempty"`
	OIDC       *AccessControlPolicyOIDC       `json:"oidc,omitempty"`
	OIDCGoogle *AccessControlPolicyOIDCGoogle `json:"oidcGoogle,omitempty"`
}

// AccessControlPolicyJWT describes the settings for JWT authentication within an access control policy.
type AccessControlPolicyJWT struct {
	SigningSecret              string            `json:"signingSecret,omitempty"`
	SigningSecretBase64Encoded bool              `json:"signingSecretBase64Encoded"`
	PublicKey                  string            `json:"publicKey,omitempty"`
	JWKsFile                   string            `json:"jwksFile,omitempty"`
	JWKsURL                    string            `json:"jwksUrl,omitempty"`
	StripAuthorizationHeader   bool              `json:"stripAuthorizationHeader,omitempty"`
	ForwardHeaders             map[string]string `json:"forwardHeaders,omitempty"`
	TokenQueryKey              string            `json:"tokenQueryKey,omitempty"`
	Claims                     string            `json:"claims,omitempty"`
}

// AccessControlPolicyBasicAuth holds the HTTP basic authentication configuration.
type AccessControlPolicyBasicAuth struct {
	Users                    string `json:"users,omitempty"` // Redacted.
	Realm                    string `json:"realm,omitempty"`
	StripAuthorizationHeader bool   `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string `json:"forwardUsernameHeader,omitempty"`
}

// AccessControlPolicyAPIKey describes the settings for APIKey authentication within an access control policy.
type AccessControlPolicyAPIKey struct {
	Header         string                         `json:"header,omitempty"`
	Query          string                         `json:"query,omitempty"`
	Cookie         string                         `json:"cookie,omitempty"`
	Keys           []AccessControlPolicyAPIKeyKey `json:"keys,omitempty"`
	ForwardHeaders map[string]string              `json:"forwardHeaders,omitempty"`
}

// AccessControlPolicyAPIKeyKey defines an API key.
type AccessControlPolicyAPIKeyKey struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata"`
	Value    string            `json:"value"` // Redacted.
}

// AccessControlPolicyOIDC holds the OIDC configuration.
type AccessControlPolicyOIDC struct {
	Issuer   string           `json:"issuer,omitempty"`
	ClientID string           `json:"clientId,omitempty"`
	Secret   *SecretReference `json:"secret,omitempty"`

	RedirectURL string            `json:"redirectUrl,omitempty"`
	LogoutURL   string            `json:"logoutUrl,omitempty"`
	Scopes      []string          `json:"scopes,omitempty"`
	AuthParams  map[string]string `json:"authParams,omitempty"`
	StateCookie *AuthStateCookie  `json:"stateCookie,omitempty"`
	Session     *AuthSession      `json:"session,omitempty"`

	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
	Claims         string            `json:"claims,omitempty"`
}

// AccessControlPolicyOIDCGoogle holds the Google OIDC configuration.
type AccessControlPolicyOIDCGoogle struct {
	ClientID string           `json:"clientId,omitempty"`
	Secret   *SecretReference `json:"secret,omitempty"`

	RedirectURL string            `json:"redirectUrl,omitempty"`
	LogoutURL   string            `json:"logoutUrl,omitempty"`
	AuthParams  map[string]string `json:"authParams,omitempty"`
	StateCookie *AuthStateCookie  `json:"stateCookie,omitempty"`
	Session     *AuthSession      `json:"session,omitempty"`

	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
	Emails         []string          `json:"emails,omitempty"`
}

// SecretReference represents a Secret Reference.
// It has enough information to retrieve secret in any namespace.
type SecretReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// AuthStateCookie carries the state cookie configuration.
type AuthStateCookie struct {
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
}

// AuthSession carries session and session cookie configuration.
type AuthSession struct {
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Refresh  *bool  `json:"refresh,omitempty"`
}

// EdgeIngress holds the definition of an EdgeIngress configuration.
type EdgeIngress struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Status    EdgeIngressStatus  `json:"status"`
	Service   EdgeIngressService `json:"service"`
	ACP       *EdgeIngressACP    `json:"acp,omitempty"`
}

// EdgeIngressStatus is the exposition status of an edge ingress.
type EdgeIngressStatus string

// Possible value of the EdgeIngressStatus.
const (
	EdgeIngressStatusUp   EdgeIngressStatus = "up"
	EdgeIngressStatusDown EdgeIngressStatus = "down"
)

// EdgeIngressService configures the service to exposed on the edge.
type EdgeIngressService struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// EdgeIngressACP configures the ACP to use on the Ingress.
type EdgeIngressACP struct {
	Name string `json:"name"`
}
