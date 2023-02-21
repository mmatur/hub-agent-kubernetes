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
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessControlPolicy defines an access control policy.
// +kubebuilder:resource:scope=Cluster
type AccessControlPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessControlPolicySpec `json:"spec,omitempty"`

	// The current status of this access control policy.
	// +optional
	Status AccessControlPolicyStatus `json:"status,omitempty"`
}

// AccessControlPolicySpec configures an access control policy.
type AccessControlPolicySpec struct {
	JWT        *AccessControlPolicyJWT        `json:"jwt,omitempty"`
	BasicAuth  *AccessControlPolicyBasicAuth  `json:"basicAuth,omitempty"`
	APIKey     *AccessControlPolicyAPIKey     `json:"apiKey,omitempty"`
	OIDC       *AccessControlPolicyOIDC       `json:"oidc,omitempty"`
	OIDCGoogle *AccessControlPolicyOIDCGoogle `json:"oidcGoogle,omitempty"`
	OAuthIntro *AccessControlOAuthIntro       `json:"oAuthIntro,omitempty"`
}

// Hash return AccessControlPolicySpec hash.
func (a AccessControlPolicySpec) Hash() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("encode ACP spec: %w", err)
	}

	hash := sha1.New()
	hash.Write(b)

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}

// AccessControlPolicyJWT configures a JWT access control policy.
type AccessControlPolicyJWT struct {
	SigningSecret              string            `json:"signingSecret,omitempty"`
	SigningSecretBase64Encoded bool              `json:"signingSecretBase64Encoded,omitempty"`
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
	Users                    []string `json:"users,omitempty"`
	Realm                    string   `json:"realm,omitempty"`
	StripAuthorizationHeader bool     `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string   `json:"forwardUsernameHeader,omitempty"`
}

// AccessControlPolicyAPIKey configure an APIKey control policy.
type AccessControlPolicyAPIKey struct {
	// Header name where to look for the API key. One of header, query or cookie must be set.
	Header string `json:"header,omitempty"`
	// Query name where to look for the API key. One of header, query or cookie must be set.
	Query string `json:"query,omitempty"`
	// Cookie name where to look for the API key. One of header, query or cookie must be set.
	Cookie string `json:"cookie,omitempty"`
	// Keys define the set of authorized keys to access a protected resource.
	// +kubebuilder:validation:MinItems:=1
	Keys []AccessControlPolicyAPIKeyKey `json:"keys,omitempty"`
	// ForwardHeaders instructs the middleware to forward key metadata as header values upon successful authentication.
	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
}

// AccessControlPolicyAPIKeyKey defines an API key.
type AccessControlPolicyAPIKeyKey struct {
	// ID is the unique identifier of the key.
	// +kubebuilder:validation:Required
	ID string `json:"id"`
	// Value is the SHAKE-256 hash (using 64 bytes) of the API key.
	// +kubebuilder:validation:Required
	Value string `json:"value"`
	// Metadata holds arbitrary metadata for this key, can be used by ForwardHeaders.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AccessControlPolicyOIDC holds the OIDC authentication configuration.
type AccessControlPolicyOIDC struct {
	Issuer   string `json:"issuer,omitempty"`
	ClientID string `json:"clientId,omitempty"`

	Secret *corev1.SecretReference `json:"secret,omitempty"`

	RedirectURL string            `json:"redirectUrl,omitempty"`
	LogoutURL   string            `json:"logoutUrl,omitempty"`
	AuthParams  map[string]string `json:"authParams,omitempty"`

	StateCookie *StateCookie `json:"stateCookie,omitempty"`
	Session     *Session     `json:"session,omitempty"`

	Scopes         []string          `json:"scopes,omitempty"`
	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
	Claims         string            `json:"claims,omitempty"`
}

// AccessControlPolicyOIDCGoogle holds the Google OIDC authentication configuration.
type AccessControlPolicyOIDCGoogle struct {
	ClientID string `json:"clientId,omitempty"`

	Secret *corev1.SecretReference `json:"secret,omitempty"`

	RedirectURL string            `json:"redirectUrl,omitempty"`
	LogoutURL   string            `json:"logoutUrl,omitempty"`
	AuthParams  map[string]string `json:"authParams,omitempty"`

	StateCookie *StateCookie `json:"stateCookie,omitempty"`
	Session     *Session     `json:"session,omitempty"`

	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
	// Emails are the allowed emails to connect.
	// +kubebuilder:validation:MinItems:=1
	Emails []string `json:"emails,omitempty"`
}

// StateCookie holds state cookie configuration.
type StateCookie struct {
	SameSite string `json:"sameSite,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
}

// Session holds session configuration.
type Session struct {
	SameSite string `json:"sameSite,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Refresh  *bool  `json:"refresh,omitempty"`
}

// AccessControlOAuthIntro configures an OAuth 2.0 Token Introspection access control policy.
type AccessControlOAuthIntro struct {
	// +kubebuilder:validation:Required
	ClientConfig AccessControlOAuthIntroClientConfig `json:"clientConfig"`
	// +kubebuilder:validation:Required
	TokenSource    TokenSource       `json:"tokenSource"`
	Claims         string            `json:"claims,omitempty"`
	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
}

// AccessControlOAuthIntroClientConfig configures the OAuth 2.0 client for issuing token introspection requests.
type AccessControlOAuthIntroClientConfig struct {
	HTTPClientConfig `json:",inline"`

	// URL of the Authorization Server.
	// +kubebuilder:validation:Required
	URL string `json:"url"`
	// Auth configures the required authentication to the Authorization Server.
	// +kubebuilder:validation:Required
	Auth AccessControlOAuthIntroClientConfigAuth `json:"auth"`
	// Headers to set when sending requests to the Authorization Server.
	Headers map[string]string `json:"headers,omitempty"`
	// TokenTypeHint is a hint to pass to the Authorization Server.
	// See https://tools.ietf.org/html/rfc7662#section-2.1 for more information.
	TokenTypeHint string `json:"tokenTypeHint,omitempty"`
}

// AccessControlOAuthIntroClientConfigAuth configures authentication to the Authorization Server.
type AccessControlOAuthIntroClientConfigAuth struct {
	// Kind sets the kind of authentication that can be used to authenticate requests.
	// The content of the referenced depends on this kind.
	// +kubebuilder:validation:Enum:=Basic;Bearer;Header;Query
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
	// Secret is the reference to the Kubernetes secrets containing sensitive authentication data.
	// +kubebuilder:validation:Required
	Secret corev1.SecretReference `json:"secret"`
}

// HTTPClientConfig configures HTTP clients.
type HTTPClientConfig struct {
	// TLS configures TLS communication with the Authorization Server.
	TLS *HTTPClientConfigTLS `json:"tls,omitempty"`
	// TimeoutSeconds configures the maximum amount of seconds to wait before giving up on requests.
	// +kubebuilder:default:=5
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// MaxRetries defines the number of retries for introspection requests.
	// +kubebuilder:default:=3
	MaxRetries int `json:"maxRetries,omitempty"`
}

// HTTPClientConfigTLS configures TLS for HTTP clients.
type HTTPClientConfigTLS struct {
	// CABundle sets the CA bundle used to sign the Authorization Server certificate.
	CABundle string `json:"caBundle,omitempty"`
	// InsecureSkipVerify skips the Authorization Server certificate validation.
	// For testing purposes only, do not use in production.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// TokenSource describes how to extract tokens from HTTP requests.
// If multiple sources are set, the order is the following: header > query > cookie.
type TokenSource struct {
	// Header is the name of a header.
	Header string `json:"header,omitempty"`
	// HeaderAuthScheme sets an optional auth scheme when Header is set to "Authorization".
	// If set, this scheme is removed from the token, and all requests not including it are dropped.
	HeaderAuthScheme string `json:"headerAuthScheme,omitempty"`
	// Query is the name of a query parameter.
	Query string `json:"query,omitempty"`
	// Cookie is the name of a cookie.
	Cookie string `json:"cookie,omitempty"`
}

// AccessControlPolicyStatus is the status of the access control policy.
type AccessControlPolicyStatus struct {
	Version  string      `json:"version,omitempty"`
	SyncedAt metav1.Time `json:"syncedAt,omitempty"`
	SpecHash string      `json:"specHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessControlPolicyList defines a list of access control policy.
type AccessControlPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []AccessControlPolicy `json:"items"`
}
