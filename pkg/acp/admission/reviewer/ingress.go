package reviewer

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/traefik/hub-agent/pkg/acp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AnnotationHubAuth is the annotation to add to an Ingress resource in order to enable Hub authentication.
const AnnotationHubAuth = "hub.traefik.io/access-control-policy"

// Ingress controller default annotations.
const (
	defaultAnnotationHAProxy = "haproxy"
	defaultAnnotationNginx   = "nginx"
	defaultAnnotationTraefik = "traefik"
)

// ingress is a generic form of netv1, netv1beta1 and extv1 ingress resources.
type ingress struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     ingressSpec       `json:"spec"`
}

type ingressSpec struct {
	Rules []ingressRule `json:"rules"`
}

type ingressRule struct {
	Host string           `json:"host"`
	HTTP *ingressRuleHTTP `json:"http"`
}

type ingressRuleHTTP struct {
	Paths []interface{} `json:"paths"`
}

// parseRawIngresses parses raw objects from admission requests into generic ingress resources.
func parseRawIngresses(newRaw, oldRaw []byte) (newIng, oldIng ingress, err error) {
	if err = json.Unmarshal(newRaw, &newIng); err != nil {
		return ingress{}, ingress{}, fmt.Errorf("unmarshal reviewed ingress: %w", err)
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldIng); err != nil {
			return ingress{}, ingress{}, fmt.Errorf("unmarshal reviewed old ingress: %w", err)
		}
	}

	return newIng, oldIng, nil
}

// countRoutes counts the number of routes exposed by a single ingress.
// An ingress with no rule counts as 1 route.
func countRoutes(spec ingressSpec) int {
	var count int
	if len(spec.Rules) == 0 {
		return 1
	}

	for _, rule := range spec.Rules {
		if rule.HTTP == nil {
			count++
			continue
		}

		count += len(rule.HTTP.Paths)
	}

	return count
}

func resourceID(name, ns string) string {
	return name + "@" + ns
}

// parseIngressClass parses a raw, JSON-marshaled ingress and returns the ingress class it refers to.
func parseIngressClass(obj []byte) (ingClassName, ingClassAnno string, err error) {
	var ing struct {
		ObjectMeta struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			IngressClassName string `json:"ingressClassName"`
		} `json:"spec"`
	}
	if err = json.Unmarshal(obj, &ing); err != nil {
		return "", "", err
	}

	return ing.Spec.IngressClassName, ing.ObjectMeta.Annotations["kubernetes.io/ingress.class"], nil
}

func headerToForward(cfg *acp.Config) ([]string, error) {
	var headerToFwd []string
	switch {
	case cfg.JWT != nil:
		for headerName := range cfg.JWT.ForwardHeaders {
			headerToFwd = append(headerToFwd, headerName)
		}
		if cfg.JWT.StripAuthorizationHeader {
			headerToFwd = append(headerToFwd, "Authorization")
		}
	case cfg.BasicAuth != nil:
		if headerName := cfg.BasicAuth.ForwardUsernameHeader; headerName != "" {
			headerToFwd = append(headerToFwd, headerName)
		}
		if cfg.BasicAuth.StripAuthorizationHeader {
			headerToFwd = append(headerToFwd, "Authorization")
		}
	case cfg.DigestAuth != nil:
		if headerName := cfg.DigestAuth.ForwardUsernameHeader; headerName != "" {
			headerToFwd = append(headerToFwd, headerName)
		}
		if cfg.DigestAuth.StripAuthorizationHeader {
			headerToFwd = append(headerToFwd, "Authorization")
		}
	default:
		return nil, errors.New("unsupported ACP type")
	}
	return headerToFwd, nil
}

func releaseQuotas(q QuotaTransaction, name, ns string) error {
	tx, err := q.Tx(resourceID(name, ns), 0)
	if err != nil {
		return err
	}

	tx.Commit()

	return nil
}

func isDefaultIngressClassValue(value string) bool {
	switch value {
	case defaultAnnotationHAProxy, defaultAnnotationTraefik, defaultAnnotationNginx:
		return true
	default:
		return false
	}
}
