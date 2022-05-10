package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressClass defines an ingress class.
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Controller",type=string,JSONPath=`.spec.controller`
// +kubebuilder:printcolumn:name="Is Default",type=string,JSONPath=`.metadata.annotations.ingressclass\.kubernetes\.io/is-default-class`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type IngressClass struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec IngressClassSpec `json:"spec,omitempty"`
}

// IngressClassSpec configures an ingress class.
type IngressClassSpec struct {
	Controller string `json:"controller"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IngressClassList defines a list of ingress class.
type IngressClassList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `son:"metadata,omitempty"`

	Items []IngressClass `json:"items"`
}
