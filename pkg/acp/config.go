/*
Copyright (C) 2022-2023 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package acp

import (
	"fmt"
	"strings"

	"github.com/traefik/hub-agent-kubernetes/pkg/acp/apikey"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/oidc"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
)

// Config is the configuration of an Access Control Policy. It is used to set up ACP handlers.
type Config struct {
	JWT        *jwt.Config       `json:"jwt"`
	BasicAuth  *basicauth.Config `json:"basicAuth"`
	APIKey     *apikey.Config    `json:"apiKey"`
	OIDC       *oidc.Config      `json:"oidc"`
	OIDCGoogle *OIDCGoogle       `json:"oidcGoogle"`
}

// OIDCGoogle is the Google OIDC configuration.
type OIDCGoogle struct {
	oidc.Config

	Emails []string `json:"emails,omitempty"`
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

	case policy.Spec.APIKey != nil:
		apiKeyConfig := policy.Spec.APIKey

		keys := make([]apikey.Key, 0, len(apiKeyConfig.Keys))
		for _, k := range apiKeyConfig.Keys {
			keys = append(keys, apikey.Key{
				ID:       k.ID,
				Metadata: k.Metadata,
				Value:    k.Value,
			})
		}

		return &Config{
			APIKey: &apikey.Config{
				Header:         apiKeyConfig.Header,
				Query:          apiKeyConfig.Query,
				Cookie:         apiKeyConfig.Cookie,
				Keys:           keys,
				ForwardHeaders: apiKeyConfig.ForwardHeaders,
			},
		}

	case policy.Spec.OIDC != nil:
		oidcCfg := policy.Spec.OIDC

		conf := &Config{
			OIDC: &oidc.Config{
				Issuer:         oidcCfg.Issuer,
				ClientID:       oidcCfg.ClientID,
				RedirectURL:    oidcCfg.RedirectURL,
				LogoutURL:      oidcCfg.LogoutURL,
				Scopes:         oidcCfg.Scopes,
				AuthParams:     oidcCfg.AuthParams,
				ForwardHeaders: oidcCfg.ForwardHeaders,
				Claims:         oidcCfg.Claims,
			},
		}

		if oidcCfg.Secret != nil {
			conf.OIDC.Secret = &oidc.SecretReference{
				Name:      oidcCfg.Secret.Name,
				Namespace: oidcCfg.Secret.Namespace,
			}
		}

		if oidcCfg.StateCookie != nil {
			conf.OIDC.StateCookie = &oidc.AuthStateCookie{
				Path:     oidcCfg.StateCookie.Path,
				Domain:   oidcCfg.StateCookie.Domain,
				SameSite: oidcCfg.StateCookie.SameSite,
				Secure:   oidcCfg.StateCookie.Secure,
			}
		}

		if oidcCfg.Session != nil {
			conf.OIDC.Session = &oidc.AuthSession{
				Path:     oidcCfg.Session.Path,
				Domain:   oidcCfg.Session.Domain,
				SameSite: oidcCfg.Session.SameSite,
				Secure:   oidcCfg.Session.Secure,
				Refresh:  oidcCfg.Session.Refresh,
			}
		}

		return conf

	case policy.Spec.OIDCGoogle != nil:
		oidcGoogleCfg := policy.Spec.OIDCGoogle

		conf := &Config{
			OIDCGoogle: &OIDCGoogle{
				Config: oidc.Config{
					Issuer:         "https://accounts.google.com",
					ClientID:       oidcGoogleCfg.ClientID,
					RedirectURL:    oidcGoogleCfg.RedirectURL,
					LogoutURL:      oidcGoogleCfg.LogoutURL,
					Scopes:         []string{"email"},
					AuthParams:     oidcGoogleCfg.AuthParams,
					ForwardHeaders: oidcGoogleCfg.ForwardHeaders,
					Claims:         buildClaims(oidcGoogleCfg.Emails),
				},
				Emails: oidcGoogleCfg.Emails,
			},
		}

		if oidcGoogleCfg.Secret != nil {
			conf.OIDCGoogle.Secret = &oidc.SecretReference{
				Name:      oidcGoogleCfg.Secret.Name,
				Namespace: oidcGoogleCfg.Secret.Namespace,
			}
		}

		if oidcGoogleCfg.StateCookie != nil {
			conf.OIDCGoogle.StateCookie = &oidc.AuthStateCookie{
				Path:     oidcGoogleCfg.StateCookie.Path,
				Domain:   oidcGoogleCfg.StateCookie.Domain,
				SameSite: oidcGoogleCfg.StateCookie.SameSite,
				Secure:   oidcGoogleCfg.StateCookie.Secure,
			}
		}

		if oidcGoogleCfg.StateCookie != nil {
			conf.OIDCGoogle.Session = &oidc.AuthSession{
				Path:     oidcGoogleCfg.Session.Path,
				Domain:   oidcGoogleCfg.Session.Domain,
				SameSite: oidcGoogleCfg.Session.SameSite,
				Secure:   oidcGoogleCfg.Session.Secure,
				Refresh:  oidcGoogleCfg.Session.Refresh,
			}
		}

		return conf
	default:
		return &Config{}
	}
}

// buildClaims builds the claims from the emails.
func buildClaims(emails []string) string {
	var matchers []string
	for _, email := range emails {
		matchers = append(matchers, fmt.Sprintf(`Equals("email", %q)`, email))
	}

	return strings.Join(matchers, " || ")
}
