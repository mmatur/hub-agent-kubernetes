package edgeingress

import (
	"fmt"
	"time"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EdgeIngress is an ingress exposed on the edge.
type EdgeIngress struct {
	WorkspaceID string `json:"workspaceId"`
	ClusterID   string `json:"clusterId"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`

	Domain string `json:"domain"`

	Version      string `json:"version"`
	ServiceName  string `json:"serviceName"`
	ServicePort  int    `json:"servicePort"`
	ACPName      string `json:"acpName"`
	ACPNamespace string `json:"acpNamespace"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Resource builds the v1alpha1 EdgeIngress resource.
func (e *EdgeIngress) Resource() (*hubv1alpha1.EdgeIngress, error) {
	spec := hubv1alpha1.EdgeIngressSpec{
		Service: hubv1alpha1.EdgeIngressService{
			Name: e.ServiceName,
			Port: e.ServicePort,
		},
	}

	if e.ACPName != "" {
		spec.ACP = &hubv1alpha1.EdgeIngressACP{
			Name:      e.ACPName,
			Namespace: e.ACPNamespace,
		}
	}

	specHash, err := spec.Hash()
	if err != nil {
		return nil, fmt.Errorf("compute spec hash: %w", err)
	}

	return &hubv1alpha1.EdgeIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      e.Name,
			Namespace: e.Namespace,
		},
		Spec: spec,
		Status: hubv1alpha1.EdgeIngressStatus{
			Version:    e.Version,
			SyncedAt:   metav1.Now(),
			Domain:     e.Domain,
			URL:        "https://" + e.Domain,
			Connection: hubv1alpha1.EdgeIngressConnectionDown,
			SpecHash:   specHash,
		},
	}, nil
}
