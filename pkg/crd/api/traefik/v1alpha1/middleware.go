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
	ForwardAuth *ForwardAuth `json:"forwardAuth,omitempty"`
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
