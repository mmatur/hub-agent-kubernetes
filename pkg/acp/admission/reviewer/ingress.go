package reviewer

import "encoding/json"

// AnnotationNeoAuth is the annotation to add to an Ingress resource in order to enable Neo authentication.
const AnnotationNeoAuth = "neo.traefik.io/access-control-policy"

// Ingress controller default annotations.
const (
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
