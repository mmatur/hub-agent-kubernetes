package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessControlPolicy defines an access control policy.
type AccessControlPolicy struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessControlPolicySpec `json:"spec,omitempty"`
}

// AccessControlPolicySpec configures an access control policy.
type AccessControlPolicySpec struct {
	JWT        *AccessControlPolicyJWT        `json:"jwt,omitempty"`
	BasicAuth  *AccessControlPolicyBasicAuth  `json:"basicAuth,omitempty"`
	DigestAuth *AccessControlPolicyDigestAuth `json:"digestAuth,omitempty"`
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

// AccessControlPolicyDigestAuth holds the HTTP digest authentication configuration.
type AccessControlPolicyDigestAuth struct {
	Users                    []string `json:"users,omitempty"`
	Realm                    string   `json:"realm,omitempty"`
	StripAuthorizationHeader bool     `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string   `json:"forwardUsernameHeader,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessControlPolicyList defines a list of access control policy.
type AccessControlPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []AccessControlPolicy `json:"items"`
}
