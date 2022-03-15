package acp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/digestauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config is the configuration of an Access Control Policy. It is used to setup ACP handlers.
type Config struct {
	JWT        *jwt.Config
	BasicAuth  *basicauth.Config
	DigestAuth *digestauth.Config
}

// ConfigFromPolicy returns an ACP configuration for the given policy.
func ConfigFromPolicy(policy *hubv1alpha1.AccessControlPolicy) *Config {
	switch {
	case policy.Spec.JWT != nil:
		jwtCfg := policy.Spec.JWT

		return &Config{
			JWT: &jwt.Config{
				SigningSecret:              jwtCfg.SigningSecret,
				SigningSecretBase64Encoded: jwtCfg.SigningSecretBase64Encoded,
				PublicKey:                  jwtCfg.PublicKey,
				JWKsFile:                   jwt.FileOrContent(jwtCfg.JWKsFile),
				JWKsURL:                    jwtCfg.JWKsURL,
				StripAuthorizationHeader:   jwtCfg.StripAuthorizationHeader,
				ForwardHeaders:             jwtCfg.ForwardHeaders,
				TokenQueryKey:              jwtCfg.TokenQueryKey,
				Claims:                     jwtCfg.Claims,
			},
		}

	case policy.Spec.BasicAuth != nil:
		basicCfg := policy.Spec.BasicAuth

		return &Config{
			BasicAuth: &basicauth.Config{
				Users:                    strings.Split(basicCfg.Users, ","),
				Realm:                    basicCfg.Realm,
				StripAuthorizationHeader: basicCfg.StripAuthorizationHeader,
				ForwardUsernameHeader:    basicCfg.ForwardUsernameHeader,
			},
		}

	case policy.Spec.DigestAuth != nil:
		digestCfg := policy.Spec.DigestAuth

		return &Config{
			DigestAuth: &digestauth.Config{
				Users:                    strings.Split(digestCfg.Users, ","),
				Realm:                    digestCfg.Realm,
				StripAuthorizationHeader: digestCfg.StripAuthorizationHeader,
				ForwardUsernameHeader:    digestCfg.ForwardUsernameHeader,
			},
		}

	default:
		return &Config{}
	}
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
