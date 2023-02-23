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
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/traefik/hub-agent-kubernetes/pkg/acp/apikey"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/oauthintro"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/oidc"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/httpclient"
	"github.com/traefik/hub-agent-kubernetes/pkg/optional"
	corev1 "k8s.io/api/core/v1"
)

// Config is the configuration of an Access Control Policy. It is used to set up ACP handlers.
type Config struct {
	JWT        *jwt.Config        `json:"jwt,omitempty"`
	BasicAuth  *basicauth.Config  `json:"basicAuth,omitempty"`
	APIKey     *apikey.Config     `json:"apiKey,omitempty"`
	OIDC       *oidc.Config       `json:"oidc,omitempty"`
	OIDCGoogle *OIDCGoogle        `json:"oidcGoogle,omitempty"`
	OAuthIntro *oauthintro.Config `json:"oAuthIntro,omitempty"`
}

// OIDCGoogle is the Google OIDC configuration.
type OIDCGoogle struct {
	oidc.Config

	Emails []string `json:"emails,omitempty"`
}

// SecretGetter allows getting secrets.
type SecretGetter interface {
	GetValue(secret *corev1.SecretReference, key string) ([]byte, error)
}

// ConfigFromPolicy returns an ACP configuration for the given policy without resolving secret references.
func ConfigFromPolicy(policy *hubv1alpha1.AccessControlPolicy) *Config {
	// This will never raise an error as we are not resolving secret references.
	cfg, _ := ConfigFromPolicyWithSecret(policy, emptySecretGetter{})
	return cfg
}

// ConfigFromPolicyWithSecret returns an ACP configuration for the given policy and resolves its secret references.
func ConfigFromPolicyWithSecret(policy *hubv1alpha1.AccessControlPolicy, secrets SecretGetter) (*Config, error) {
	switch {
	case policy.Spec.JWT != nil:
		return makeJWTConfig(policy.Spec.JWT), nil

	case policy.Spec.BasicAuth != nil:
		return makeBasicAuthConfig(policy.Spec.BasicAuth), nil

	case policy.Spec.APIKey != nil:
		return makeAPIKeyConfig(policy.Spec.APIKey), nil

	case policy.Spec.OIDC != nil:
		return makeOIDCConfig(policy.Spec.OIDC, secrets)

	case policy.Spec.OIDCGoogle != nil:
		return makeOIDCGoogleConfig(policy.Spec.OIDCGoogle, secrets)

	case policy.Spec.OAuthIntro != nil:
		return makeOAuthIntro(policy.Spec.OAuthIntro, secrets)
	}

	return nil, errors.New(`exactly one of "jwt", "basicAuth", "apiKey", "oidc", "oidcGoogle" or "oAuthIntro" must be set`)
}

// buildClaims builds the claims from the emails.
func buildClaims(emails []string) string {
	var matchers []string
	for _, email := range emails {
		matchers = append(matchers, fmt.Sprintf(`Equals("email", %q)`, email))
	}

	return strings.Join(matchers, " || ")
}

func makeJWTConfig(policy *hubv1alpha1.AccessControlPolicyJWT) *Config {
	return &Config{
		JWT: &jwt.Config{
			SigningSecret:              policy.SigningSecret,
			SigningSecretBase64Encoded: policy.SigningSecretBase64Encoded,
			PublicKey:                  policy.PublicKey,
			JWKsFile:                   jwt.FileOrContent(policy.JWKsFile),
			JWKsURL:                    policy.JWKsURL,
			StripAuthorizationHeader:   policy.StripAuthorizationHeader,
			ForwardHeaders:             policy.ForwardHeaders,
			TokenQueryKey:              policy.TokenQueryKey,
			Claims:                     policy.Claims,
		},
	}
}

func makeBasicAuthConfig(policy *hubv1alpha1.AccessControlPolicyBasicAuth) *Config {
	return &Config{
		BasicAuth: &basicauth.Config{
			Users:                    policy.Users,
			Realm:                    policy.Realm,
			StripAuthorizationHeader: policy.StripAuthorizationHeader,
			ForwardUsernameHeader:    policy.ForwardUsernameHeader,
		},
	}
}

func makeAPIKeyConfig(policy *hubv1alpha1.AccessControlPolicyAPIKey) *Config {
	keys := make([]apikey.Key, 0, len(policy.Keys))
	for _, k := range policy.Keys {
		keys = append(keys, apikey.Key{
			ID:       k.ID,
			Metadata: k.Metadata,
			Value:    k.Value,
		})
	}

	return &Config{
		APIKey: &apikey.Config{
			Header:         policy.Header,
			Query:          policy.Query,
			Cookie:         policy.Cookie,
			Keys:           keys,
			ForwardHeaders: policy.ForwardHeaders,
		},
	}
}

func makeOIDCConfig(policy *hubv1alpha1.AccessControlPolicyOIDC, secrets SecretGetter) (*Config, error) {
	oidcConfig := &oidc.Config{
		Issuer:         policy.Issuer,
		ClientID:       policy.ClientID,
		RedirectURL:    policy.RedirectURL,
		LogoutURL:      policy.LogoutURL,
		Scopes:         policy.Scopes,
		AuthParams:     policy.AuthParams,
		ForwardHeaders: policy.ForwardHeaders,
		Claims:         policy.Claims,
	}

	if policy.Secret != nil {
		oidcConfig.Secret = &oidc.SecretReference{
			Name:      policy.Secret.Name,
			Namespace: policy.Secret.Namespace,
		}

		clientSecret, err := secrets.GetValue(policy.Secret, "clientSecret")
		if err != nil {
			return nil, fmt.Errorf("getting client secret: %w", err)
		}

		oidcConfig.ClientSecret = string(clientSecret)
	}

	if policy.StateCookie != nil {
		oidcConfig.StateCookie = &oidc.AuthStateCookie{
			Path:     policy.StateCookie.Path,
			Domain:   policy.StateCookie.Domain,
			SameSite: policy.StateCookie.SameSite,
			Secure:   policy.StateCookie.Secure,
		}
	}

	if policy.Session != nil {
		oidcConfig.Session = &oidc.AuthSession{
			Path:     policy.Session.Path,
			Domain:   policy.Session.Domain,
			SameSite: policy.Session.SameSite,
			Secure:   policy.Session.Secure,
			Refresh:  policy.Session.Refresh,
		}
	}

	sessionSecret := &corev1.SecretReference{Namespace: currentNamespace(), Name: "hub-secret"}
	sessionKey, err := secrets.GetValue(sessionSecret, "key")
	if err != nil {
		return nil, fmt.Errorf("getting session key: %w", err)
	}

	oidcConfig.SessionKey = fmt.Sprintf("%x", sha256.Sum256(sessionKey))[:32]
	return &Config{OIDC: oidcConfig}, nil
}

func makeOIDCGoogleConfig(policy *hubv1alpha1.AccessControlPolicyOIDCGoogle, secrets SecretGetter) (*Config, error) {
	oidcGoogleConfig := &OIDCGoogle{
		Config: oidc.Config{
			Issuer:         "https://accounts.google.com",
			ClientID:       policy.ClientID,
			RedirectURL:    policy.RedirectURL,
			LogoutURL:      policy.LogoutURL,
			Scopes:         []string{"email"},
			AuthParams:     policy.AuthParams,
			ForwardHeaders: policy.ForwardHeaders,
			Claims:         buildClaims(policy.Emails),
		},
		Emails: policy.Emails,
	}

	if policy.Secret != nil {
		oidcGoogleConfig.Secret = &oidc.SecretReference{
			Name:      policy.Secret.Name,
			Namespace: policy.Secret.Namespace,
		}

		clientSecret, err := secrets.GetValue(policy.Secret, "clientSecret")
		if err != nil {
			return nil, fmt.Errorf("getting client secret: %w", err)
		}

		oidcGoogleConfig.ClientSecret = string(clientSecret)
	}

	if policy.StateCookie != nil {
		oidcGoogleConfig.StateCookie = &oidc.AuthStateCookie{
			Path:     policy.StateCookie.Path,
			Domain:   policy.StateCookie.Domain,
			SameSite: policy.StateCookie.SameSite,
			Secure:   policy.StateCookie.Secure,
		}
	}

	if policy.Session != nil {
		oidcGoogleConfig.Session = &oidc.AuthSession{
			Path:     policy.Session.Path,
			Domain:   policy.Session.Domain,
			SameSite: policy.Session.SameSite,
			Secure:   policy.Session.Secure,
			Refresh:  policy.Session.Refresh,
		}
	}

	sessionSecret := &corev1.SecretReference{Namespace: currentNamespace(), Name: "hub-secret"}
	sessionKey, err := secrets.GetValue(sessionSecret, "key")
	if err != nil {
		return nil, fmt.Errorf("getting session key: %w", err)
	}

	oidcGoogleConfig.SessionKey = fmt.Sprintf("%x", sha256.Sum256(sessionKey))[:32]
	return &Config{OIDCGoogle: oidcGoogleConfig}, nil
}

func makeOAuthIntro(policy *hubv1alpha1.AccessControlOAuthIntro, secrets SecretGetter) (*Config, error) {
	oauthIntroConfig := &oauthintro.Config{
		Claims:         policy.Claims,
		ForwardHeaders: policy.ForwardHeaders,
	}

	oauthIntroConfig.ClientConfig = oauthintro.ClientConfig{
		Config: httpclient.Config{
			TimeoutSeconds: optional.NewInt(policy.ClientConfig.TimeoutSeconds),
			MaxRetries:     optional.NewInt(policy.ClientConfig.MaxRetries),
		},
		URL:           policy.ClientConfig.URL,
		Headers:       policy.ClientConfig.Headers,
		TokenTypeHint: policy.ClientConfig.TokenTypeHint,
	}

	kind := policy.ClientConfig.Auth.Kind
	oauthIntroConfig.ClientConfig.Auth = oauthintro.ClientConfigAuth{
		Kind: kind,
	}

	oauthIntroConfig.ClientConfig.Auth.Secret = oauthintro.SecretReference{
		Name:      policy.ClientConfig.Auth.Secret.Name,
		Namespace: policy.ClientConfig.Auth.Secret.Namespace,
	}

	key, value, err := parseOAuthIntroSecret(secrets, policy.ClientConfig.Auth.Secret, kind)
	if err != nil {
		return nil, fmt.Errorf("parsing secret data: %w", err)
	}

	oauthIntroConfig.ClientConfig.Auth.Key = key
	oauthIntroConfig.ClientConfig.Auth.Value = value

	if policy.ClientConfig.TLS != nil {
		oauthIntroConfig.ClientConfig.TLS = &httpclient.ConfigTLS{
			CABundle:           policy.ClientConfig.TLS.CABundle,
			InsecureSkipVerify: policy.ClientConfig.TLS.InsecureSkipVerify,
		}
	}

	oauthIntroConfig.TokenSource = oauthintro.TokenSource{
		Header:           policy.TokenSource.Header,
		HeaderAuthScheme: policy.TokenSource.HeaderAuthScheme,
		Query:            policy.TokenSource.Query,
		Cookie:           policy.TokenSource.Cookie,
	}

	return &Config{OAuthIntro: oauthIntroConfig}, nil
}

func parseOAuthIntroSecret(secrets SecretGetter, secret corev1.SecretReference, kind string) (key, value string, err error) {
	switch kind {
	case "Bearer":
		v, err := secrets.GetValue(&secret, "value")
		if err != nil {
			return "", "", err
		}
		return "Authorization", "Bearer " + string(v), nil

	case "Basic":
		username, err := secrets.GetValue(&secret, "username")
		if err != nil {
			return "", "", err
		}

		password, err := secrets.GetValue(&secret, "password")
		if err != nil {
			return "", "", err
		}

		enc := base64.StdEncoding.EncodeToString(bytes.Join([][]byte{username, password}, []byte(":")))
		return "Authorization", "Basic " + enc, nil

	case "Header", "Query":
		k, err := secrets.GetValue(&secret, "key")
		if err != nil {
			return "", "", err
		}

		v, err := secrets.GetValue(&secret, "value")
		if err != nil {
			return "", "", err
		}

		return string(k), string(v), nil

	default:
		return "", "", fmt.Errorf("unknown kind %s", kind)
	}
}

func currentNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}

	return "default"
}
