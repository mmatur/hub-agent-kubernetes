package reviewer

import (
	"github.com/traefik/hub-agent/pkg/acp/admission/quota"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressClasses allows to get the ingress controller type given an ingress class desc
// or the default ingress controller type.
type IngressClasses interface {
	GetController(name string) (string, error)
	GetDefaultController() (string, error)
}

// QuotaTransaction allows to reserve quotas.
type QuotaTransaction interface {
	Tx(resourceID string, amount int) (*quota.Tx, error)
}

func isNetV1Ingress(resource metav1.GroupVersionKind) bool {
	return resource.Group == "networking.k8s.io" && resource.Version == "v1" && resource.Kind == "Ingress"
}

func isNetV1Beta1Ingress(resource metav1.GroupVersionKind) bool {
	return resource.Group == "networking.k8s.io" && resource.Version == "v1beta1" && resource.Kind == "Ingress"
}

func isExtV1Beta1Ingress(resource metav1.GroupVersionKind) bool {
	return resource.Group == "extensions" && resource.Version == "v1beta1" && resource.Kind == "Ingress"
}

func isTraefikV1Alpha1IngressRoute(resource metav1.GroupVersionKind) bool {
	return resource.Group == "traefik.containo.us" && resource.Version == "v1alpha1" && resource.Kind == "IngressRoute"
}
