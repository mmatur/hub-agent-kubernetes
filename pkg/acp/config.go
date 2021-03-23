package acp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/traefik/neo-agent/pkg/acp/jwt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config is the configuration of an Access Control Policy. It is used to setup ACP handlers.
type Config struct {
	JWT *jwt.Config `json:"jwt"`
}

// CanonicalName returns the canonical name of the given policy using the default namespace if none is set.
// For example:
// 		CanonicalName("foo", "bar") => "foo@bar"
// 		CanonicalName("foo@ns", "bar") => "foo@ns"
// 		CanonicalName("foo", "") => "foo@default".
func CanonicalName(polName, defaultNamespace string) (string, error) {
	if polName == "" {
		return "", errors.New("empty ACP name")
	}

	parts := strings.Split(polName, "@")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid ACP name %q, it can contain at most one '@'", polName)
	}

	ns := defaultNamespace

	if len(parts) > 1 && parts[1] != "" {
		ns = parts[1]
	}
	if ns == "" {
		ns = metav1.NamespaceDefault
	}

	return parts[0] + "@" + ns, nil
}
