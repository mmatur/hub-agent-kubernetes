package edgeingress

import "time"

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
