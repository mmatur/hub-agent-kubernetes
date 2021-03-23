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
	JWT *AccessControlPolicyJWT `json:"jwt"`
}

// AccessControlPolicyJWT configures a JWT access control policy.
type AccessControlPolicyJWT struct {
	SigningSecret              string            `json:"signingSecret"`
	SigningSecretBase64Encoded bool              `json:"signingSecretBase64Encoded"`
	PublicKey                  string            `json:"publicKey"`
	JWKsFile                   string            `json:"jwksFile"`
	JWKsURL                    string            `json:"jwksUrl"`
	StripAuthorizationHeader   bool              `json:"stripAuthorizationHeader"`
	ForwardHeaders             map[string]string `json:"forwardHeaders"`
	TokenQueryKey              string            `json:"tokenQueryKey"`
	Claims                     string            `json:"claims"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AccessControlPolicyList defines a list of access control policy.
type AccessControlPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []AccessControlPolicy `json:"items"`
}
