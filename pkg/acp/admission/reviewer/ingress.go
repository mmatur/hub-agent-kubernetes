package reviewer

import (
	"encoding/json"
	"errors"

	"github.com/traefik/neo-agent/pkg/acp"
)

// AnnotationNeoAuth is the annotation to add to an Ingress resource in order to enable Neo authentication.
const AnnotationNeoAuth = "neo.traefik.io/access-control-policy"

// Ingress controller default annotations.
const (
	defaultAnnotationHAProxy = "haproxy"
	defaultAnnotationNginx   = "nginx"
	defaultAnnotationTraefik = "traefik"
)

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
	if err := json.Unmarshal(obj, &ing); err != nil {
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
