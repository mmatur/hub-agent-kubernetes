package state

import (
	traefikv1alpha1 "github.com/traefik/hub-agent/pkg/crd/api/traefik/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/external-dns/endpoint"
)

// Cluster describes a Cluster.
type Cluster struct {
	ID                    string
	Namespaces            []string
	Apps                  map[string]*App
	Ingresses             map[string]*Ingress
	IngressRoutes         map[string]*IngressRoute `dir:"Ingresses"`
	Services              map[string]*Service
	IngressControllers    map[string]*IngressController
	ExternalDNSes         map[string]*ExternalDNS
	AccessControlPolicies map[string]*AccessControlPolicy

	TraefikServiceNames map[string]string `dir:"-"`
}

// ResourceMeta represents the metadata which identify a Kubernetes resource.
type ResourceMeta struct {
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
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

// IngressMeta represents the common Ingress metadata properties.
type IngressMeta struct {
	ClusterID   string            `json:"clusterId"`
	Controller  string            `json:"controller,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Ingress describes an Kubernetes Ingress.
type Ingress struct {
	ResourceMeta
	IngressMeta

	TLS            []netv1.IngressTLS    `json:"tls,omitempty"`
	Rules          []netv1.IngressRule   `json:"rules,omitempty"`
	DefaultBackend *netv1.IngressBackend `json:"defaultBackend,omitempty"`
	Services       []string              `json:"services,omitempty"`
}

// IngressRoute describes a Traefik IngressRoute.
type IngressRoute struct {
	ResourceMeta
	IngressMeta

	TLS      *IngressRouteTLS `json:"tls,omitempty"`
	Routes   []Route          `json:"routes,omitempty"`
	Services []string         `json:"services,omitempty"`
}

// IngressRouteTLS represents a simplified Traefik IngressRoute TLS configuration.
type IngressRouteTLS struct {
	Domains    []traefikv1alpha1.Domain `json:"domains,omitempty"`
	SecretName string                   `json:"secretName,omitempty"`
}

// Route represents a Traefik IngressRoute route.
type Route struct {
	Match    string         `json:"match"`
	Services []RouteService `json:"services,omitempty"`
}

// RouteService represents a Kubernetes service targeted by a Traefik IngressRoute route.
type RouteService struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	PortName   string `json:"portName,omitempty"`
	PortNumber int32  `json:"portNumber,omitempty"`
}

// ExternalDNS describes an External DNS configured within a cluster.
type ExternalDNS struct {
	DNSName string           `json:"dnsName"`
	Targets endpoint.Targets `json:"targets"`
	TTL     endpoint.TTL     `json:"ttl"`
}

// AccessControlPolicy describes an Access Control Policy configured within a cluster.
type AccessControlPolicy struct {
	Name       string                         `json:"name"`
	Namespace  string                         `json:"namespace"`
	ClusterID  string                         `json:"clusterId"`
	Method     string                         `json:"method"`
	JWT        *AccessControlPolicyJWT        `json:"jwt,omitempty"`
	BasicAuth  *AccessControlPolicyBasicAuth  `json:"basicAuth,omitempty"`
	DigestAuth *AccessControlPolicyDigestAuth `json:"digestAuth,omitempty"`
}

// AccessControlPolicyJWT describes the settings for JWT authentication within an access control policy.
type AccessControlPolicyJWT struct {
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

// AccessControlPolicyBasicAuth holds the HTTP basic authentication configuration.
type AccessControlPolicyBasicAuth struct {
	Users                    string `json:"users,omitempty"`
	Realm                    string `json:"realm,omitempty"`
	StripAuthorizationHeader bool   `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string `json:"forwardUsernameHeader,omitempty"`
}

// AccessControlPolicyDigestAuth holds the HTTP digest authentication configuration.
type AccessControlPolicyDigestAuth struct {
	Users                    string `json:"users,omitempty"`
	Realm                    string `json:"realm,omitempty"`
	StripAuthorizationHeader bool   `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string `json:"forwardUsernameHeader,omitempty"`
}
