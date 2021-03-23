package state

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/external-dns/endpoint"
)

// Cluster describes a Cluster.
type Cluster struct {
	ID                    string                          `json:"id"`
	Namespaces            []string                        `json:"namespaces"`
	Apps                  map[string]*App                 `json:"apps,omitempty"`
	Ingresses             map[string]*Ingress             `json:"ingresses,omitempty"`
	Services              map[string]*Service             `json:"services,omitempty"`
	IngressControllers    map[string]*IngressController   `json:"ingressControllers,omitempty"`
	ExternalDNSes         map[string]*ExternalDNS         `json:"externalDNSes,omitempty"`
	AccessControlPolicies map[string]*AccessControlPolicy `json:"accessControlPolicies,omitempty"`
}

// IngressController is an abstraction of Service that is a cluster's IngressController.
type IngressController struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	IngressClasses   []string `json:"ingressClasses,omitempty"`
	MetricsURLs      []string `json:"metricsURLs"`
	PublicIPs        []string `json:"publicIPs,omitempty"`
	ServiceAddresses []string `json:"serviceAddresses"`
	Replicas         int      `json:"replicas"`
}

// Service describes a Service.
type Service struct {
	Name      string               `json:"name"`
	Namespace string               `json:"namespace"`
	Selector  map[string]string    `json:"selector"`
	Status    corev1.ServiceStatus `json:"status"`

	addresses []string
}

// Ingress describes an Ingress.
type Ingress struct {
	Name               string                    `json:"name"`
	Namespace          string                    `json:"namespace"`
	Controller         string                    `json:"controller,omitempty"`
	Annotations        map[string]string         `json:"annotations,omitempty"`
	TLS                []IngressTLS              `json:"tls,omitempty"`
	Status             corev1.LoadBalancerStatus `json:"status"`
	Services           []string                  `json:"services"`
	CertManagerEnabled bool                      `json:"certManagerEnabled"`
}

// IngressTLS describes the transport layer security associated with an Ingress.
type IngressTLS struct {
	Hosts      []string `json:"hosts,omitempty"`
	SecretName string   `json:"secretName,omitempty"`
}

// App is an abstraction of Deployments/ReplicaSets/DaemonSets/StatefulSets.
type App struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Namespace     string            `json:"namespace"`
	Replicas      int               `json:"replicas"`
	ReadyReplicas int               `json:"readyReplicas"`
	Images        []string          `json:"images"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// ExternalDNS describes an External DNS configured within a cluster.
type ExternalDNS struct {
	DNSName string           `json:"dnsName"`
	Targets endpoint.Targets `json:"targets"`
	TTL     endpoint.TTL     `json:"ttl"`
}

// AccessControlPolicy describes an Access Control Policy configured within a cluster.
type AccessControlPolicy struct {
	JWT *JWTAccessControl `json:"jwtAccessControl,omitempty"`
}

// JWTAccessControl describes the settings for JWT authentication within an access control policy.
type JWTAccessControl struct {
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
