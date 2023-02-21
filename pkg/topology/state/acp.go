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

package state

import (
	"strings"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/httpclient"
	"github.com/traefik/hub-agent-kubernetes/pkg/optional"
	"k8s.io/apimachinery/pkg/labels"
)

const redactedValue = "redacted"

func (f *Fetcher) getAccessControlPolicies() (map[string]*AccessControlPolicy, error) {
	policies, err := f.hub.Hub().V1alpha1().AccessControlPolicies().Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	result := make(map[string]*AccessControlPolicy)
	for _, policy := range policies {
		acp := &AccessControlPolicy{
			Name: policy.Name,
		}

		switch {
		case policy.Spec.BasicAuth != nil:
			acp.Method = "basicAuth"
			acp.BasicAuth = makeAccessControlBasicAuth(policy.Spec.BasicAuth)

		case policy.Spec.JWT != nil:
			acp.Method = "jwt"
			acp.JWT = makeAccessControlPolicyJWT(policy.Spec.JWT)

		case policy.Spec.APIKey != nil:
			acp.Method = "apiKey"
			acp.APIKey = &AccessControlPolicyAPIKey{
				Header:         policy.Spec.APIKey.Header,
				Query:          policy.Spec.APIKey.Query,
				Cookie:         policy.Spec.APIKey.Cookie,
				Keys:           redactKeys(policy.Spec.APIKey.Keys),
				ForwardHeaders: policy.Spec.APIKey.ForwardHeaders,
			}

		case policy.Spec.OIDC != nil:
			acp.Method = "oidc"
			acp.OIDC = makeAccessControlOIDC(policy.Spec.OIDC)

		case policy.Spec.OIDCGoogle != nil:
			acp.Method = "oidcGoogle"
			acp.OIDCGoogle = makeAccessControlOIDCGoogle(policy.Spec.OIDCGoogle)

		case policy.Spec.OAuthIntro != nil:
			acp.Method = "oAuthIntro"
			acp.OAuthIntro = makeAccessControlPolicyOAuthIntro(policy.Spec.OAuthIntro)

		default:
			continue
		}

		result[policy.Name] = acp
	}

	return result, nil
}

func makeAccessControlBasicAuth(cfg *hubv1alpha1.AccessControlPolicyBasicAuth) *AccessControlPolicyBasicAuth {
	return &AccessControlPolicyBasicAuth{
		Users:                    redactPasswords(cfg.Users),
		Realm:                    cfg.Realm,
		StripAuthorizationHeader: cfg.StripAuthorizationHeader,
		ForwardUsernameHeader:    cfg.ForwardUsernameHeader,
	}
}

func makeAccessControlPolicyJWT(cfg *hubv1alpha1.AccessControlPolicyJWT) *AccessControlPolicyJWT {
	policy := &AccessControlPolicyJWT{
		SigningSecretBase64Encoded: cfg.SigningSecretBase64Encoded,
		PublicKey:                  cfg.PublicKey,
		JWKsFile:                   cfg.JWKsFile,
		JWKsURL:                    cfg.JWKsURL,
		StripAuthorizationHeader:   cfg.StripAuthorizationHeader,
		ForwardHeaders:             cfg.ForwardHeaders,
		TokenQueryKey:              cfg.TokenQueryKey,
		Claims:                     cfg.Claims,
	}

	if cfg.SigningSecret != "" {
		policy.SigningSecret = redactedValue
	}

	return policy
}

func makeAccessControlOIDC(cfg *hubv1alpha1.AccessControlPolicyOIDC) *AccessControlPolicyOIDC {
	policy := &AccessControlPolicyOIDC{
		Issuer:         cfg.Issuer,
		ClientID:       cfg.ClientID,
		RedirectURL:    cfg.RedirectURL,
		LogoutURL:      cfg.LogoutURL,
		Scopes:         cfg.Scopes,
		AuthParams:     cfg.AuthParams,
		ForwardHeaders: cfg.ForwardHeaders,
		Claims:         cfg.Claims,
	}

	if cfg.Secret != nil {
		policy.Secret = &SecretReference{
			Name:      cfg.Secret.Name,
			Namespace: cfg.Secret.Namespace,
		}
	}

	if cfg.StateCookie != nil {
		policy.StateCookie = &AuthStateCookie{
			Path:     cfg.StateCookie.Path,
			Domain:   cfg.StateCookie.Domain,
			SameSite: cfg.StateCookie.SameSite,
			Secure:   cfg.StateCookie.Secure,
		}
	}

	if cfg.Session != nil {
		policy.Session = &AuthSession{
			Path:     cfg.Session.Path,
			Domain:   cfg.Session.Domain,
			SameSite: cfg.Session.SameSite,
			Secure:   cfg.Session.Secure,
			Refresh:  cfg.Session.Refresh,
		}
	}

	return policy
}

func makeAccessControlOIDCGoogle(cfg *hubv1alpha1.AccessControlPolicyOIDCGoogle) *AccessControlPolicyOIDCGoogle {
	policy := &AccessControlPolicyOIDCGoogle{
		ClientID:       cfg.ClientID,
		RedirectURL:    cfg.RedirectURL,
		LogoutURL:      cfg.LogoutURL,
		AuthParams:     cfg.AuthParams,
		ForwardHeaders: cfg.ForwardHeaders,
		Emails:         cfg.Emails,
	}

	if cfg.Secret != nil {
		policy.Secret = &SecretReference{
			Name:      cfg.Secret.Name,
			Namespace: cfg.Secret.Namespace,
		}
	}

	if cfg.StateCookie != nil {
		policy.StateCookie = &AuthStateCookie{
			Path:     cfg.StateCookie.Path,
			Domain:   cfg.StateCookie.Domain,
			SameSite: cfg.StateCookie.SameSite,
			Secure:   cfg.StateCookie.Secure,
		}
	}

	if cfg.Session != nil {
		policy.Session = &AuthSession{
			Path:     cfg.Session.Path,
			Domain:   cfg.Session.Domain,
			SameSite: cfg.Session.SameSite,
			Secure:   cfg.Session.Secure,
			Refresh:  cfg.Session.Refresh,
		}
	}

	return policy
}

func makeAccessControlPolicyOAuthIntro(cfg *hubv1alpha1.AccessControlOAuthIntro) *AccessControlPolicyOAuthIntro {
	policy := &AccessControlPolicyOAuthIntro{
		Claims:         cfg.Claims,
		ForwardHeaders: cfg.ForwardHeaders,
	}

	policy.ClientConfig = ClientConfig{
		Config: httpclient.Config{
			TimeoutSeconds: optional.NewInt(cfg.ClientConfig.TimeoutSeconds),
			MaxRetries:     optional.NewInt(cfg.ClientConfig.MaxRetries),
		},
		URL:           cfg.ClientConfig.URL,
		Headers:       cfg.ClientConfig.Headers,
		TokenTypeHint: cfg.ClientConfig.TokenTypeHint,
	}

	policy.ClientConfig.Auth = ClientConfigAuth{
		Kind: cfg.ClientConfig.Auth.Kind,
	}

	policy.ClientConfig.Auth.Secret = SecretReference{
		Name:      cfg.ClientConfig.Auth.Secret.Name,
		Namespace: cfg.ClientConfig.Auth.Secret.Namespace,
	}

	if cfg.ClientConfig.TLS != nil {
		policy.ClientConfig.TLS = &httpclient.ConfigTLS{
			CABundle:           cfg.ClientConfig.TLS.CABundle,
			InsecureSkipVerify: cfg.ClientConfig.TLS.InsecureSkipVerify,
		}
	}

	policy.TokenSource = TokenSource{
		Header:           cfg.TokenSource.Header,
		HeaderAuthScheme: cfg.TokenSource.HeaderAuthScheme,
		Query:            cfg.TokenSource.Query,
		Cookie:           cfg.TokenSource.Cookie,
	}

	return policy
}

func redactPasswords(rawUsers []string) string {
	var users []string

	for _, u := range rawUsers {
		i := strings.Index(u, ":")
		if i <= 0 {
			continue
		}

		users = append(users, u[:i]+":"+redactedValue)
	}

	return strings.Join(users, ",")
}

func redactKeys(keys []hubv1alpha1.AccessControlPolicyAPIKeyKey) []AccessControlPolicyAPIKeyKey {
	out := make([]AccessControlPolicyAPIKeyKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, AccessControlPolicyAPIKeyKey{
			ID:       key.ID,
			Metadata: key.Metadata,
			Value:    "redacted",
		})
	}
	return out
}
