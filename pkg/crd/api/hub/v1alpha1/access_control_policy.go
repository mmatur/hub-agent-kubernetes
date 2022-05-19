package v1alpha1

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"

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
	DigestAuth *AccessControlPolicyDigestAuth `json:"digestAuth,omitempty"`
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

// AccessControlPolicyDigestAuth holds the HTTP digest authentication configuration.
type AccessControlPolicyDigestAuth struct {
	Users                    []string `json:"users,omitempty"`
	Realm                    string   `json:"realm,omitempty"`
	StripAuthorizationHeader bool     `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string   `json:"forwardUsernameHeader,omitempty"`
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
