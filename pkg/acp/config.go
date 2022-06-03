package acp

import (
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
)

// Config is the configuration of an Access Control Policy. It is used to setup ACP handlers.
type Config struct {
	JWT       *jwt.Config
	BasicAuth *basicauth.Config
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
				Users:                    basicCfg.Users,
				Realm:                    basicCfg.Realm,
				StripAuthorizationHeader: basicCfg.StripAuthorizationHeader,
				ForwardUsernameHeader:    basicCfg.ForwardUsernameHeader,
			},
		}

	default:
		return &Config{}
	}
}
