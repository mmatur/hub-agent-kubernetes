package state

import (
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/external-dns/endpoint"
)

// Cluster describes a Cluster.
type Cluster struct {
	ID                    string                          `json:"id"`
	Namespaces            []string                        `json:"namespaces,omitempty"`
	Apps                  map[string]*App                 `json:"apps,omitempty"`
	Ingresses             map[string]*Ingress             `json:"ingresses,omitempty"`
	Services              map[string]*Service             `json:"services,omitempty"`
	IngressControllers    map[string]*IngressController   `json:"ingressControllers,omitempty"`
	ExternalDNSes         map[string]*ExternalDNS         `json:"externalDNSes,omitempty"`
	AccessControlPolicies map[string]*AccessControlPolicy `json:"accessControlPolicies,omitempty"`
}

// App is an abstraction of Deployments/ReplicaSets/DaemonSets/StatefulSets.
type App struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Namespace     string            `json:"namespace"`
	Replicas      int               `json:"replicas"`
	ReadyReplicas int               `json:"readyReplicas"`
	Images        []string          `json:"images,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`

	podLabels map[string]string
}

// IngressController is an abstraction of Deployments/ReplicaSets/DaemonSets/StatefulSets that
// are a cluster's IngressController.
type IngressController struct {
	App

	Type           string   `json:"type"`
	IngressClasses []string `json:"ingressClasses,omitempty"`
	MetricsURLs    []string `json:"metricsURLs,omitempty"`
	PublicIPs      []string `json:"publicIPs,omitempty"`
}

// Service describes a Service.
type Service struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Type      corev1.ServiceType `json:"type"`
	Selector  map[string]string  `json:"selector"`
	Apps      []string           `json:"apps,omitempty"`

	status corev1.ServiceStatus
}

// Ingress describes an Ingress.
type Ingress struct {
	Name           string                `json:"name"`
	Namespace      string                `json:"namespace"`
	ClusterID      string                `json:"clusterID"`
	Controller     string                `json:"controller,omitempty"`
	Annotations    map[string]string     `json:"annotations,omitempty"`
	TLS            []netv1.IngressTLS    `json:"tls,omitempty"`
	Rules          []netv1.IngressRule   `json:"rules,omitempty"`
	DefaultService *netv1.IngressBackend `json:"defaultService,omitempty"`
	Services       []string              `json:"services,omitempty"`
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
