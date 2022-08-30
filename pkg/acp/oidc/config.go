/*
Copyright (C) 2022 Traefik Labs

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

package oidc

import (
	"errors"
)

// Config holds the configuration for the OIDC middleware.
type Config struct {
	Issuer       string           `json:"issuer,omitempty"`
	ClientID     string           `json:"clientId,omitempty"`
	ClientSecret string           `json:"-"`
	Secret       *SecretReference `json:"secret,omitempty"`

	RedirectURL string            `json:"redirectUrl,omitempty"`
	LogoutURL   string            `json:"logoutUrl,omitempty"`
	Scopes      []string          `json:"scopes,omitempty"`
	AuthParams  map[string]string `json:"authParams,omitempty"`
	StateCookie *AuthStateCookie  `json:"stateCookie,omitempty"`
	Session     *AuthSession      `json:"session,omitempty"`

	// ForwardHeaders defines headers that should be added to the request and populated with values extracted from the ID token.
	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
	// Claims defines an expression to perform validation on the ID token. For example:
	//     Equals(`grp`, `admin`) && Equals(`scope`, `deploy`)
	Claims string `json:"claims,omitempty"`
}

// ApplyDefaultValues applies default values on the given dynamic configuration.
func (cfg *Config) ApplyDefaultValues() {
	if cfg == nil {
		return
	}

	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid"}
	}

	if cfg.StateCookie == nil {
		cfg.StateCookie = &AuthStateCookie{}
	}

	if cfg.StateCookie.Path == "" {
		cfg.StateCookie.Path = "/"
	}

	if cfg.StateCookie.SameSite == "" {
		cfg.StateCookie.SameSite = "lax"
	}

	if cfg.Session == nil {
		cfg.Session = &AuthSession{}
	}

	if cfg.Session.Path == "" {
		cfg.Session.Path = "/"
	}

	if cfg.Session.SameSite == "" {
		cfg.Session.SameSite = "lax"
	}

	if cfg.Session.Refresh == nil {
		cfg.Session.Refresh = ptrBool(true)
	}

	if cfg.RedirectURL == "" {
		cfg.RedirectURL = "/callback"
	}
}

// Validate validates configuration.
func (cfg *Config) Validate() error {
	if cfg == nil {
		return nil
	}

	cfg.ApplyDefaultValues()

	if cfg.Issuer == "" {
		return errors.New("missing issuer")
	}

	if cfg.ClientID == "" {
		return errors.New("missing client ID")
	}

	if cfg.ClientSecret == "" {
		return errors.New("missing client secret")
	}

	if cfg.Session.Secret == "" {
		return errors.New("missing session secret")
	}

	switch len(cfg.Session.Secret) {
	case 16, 24, 32:
		break
	default:
		return errors.New("session secret must be 16, 24 or 32 characters long")
	}

	if cfg.StateCookie.Secret == "" {
		return errors.New("missing state secret")
	}

	switch len(cfg.StateCookie.Secret) {
	case 16, 24, 32:
		break
	default:
		return errors.New("state secret must be 16, 24 or 32 characters long")
	}

	if cfg.RedirectURL == "" {
		return errors.New("missing redirect URL")
	}

	return nil
}

// SecretReference represents a Secret Reference.
// It has enough information to retrieve secret in any namespace.
type SecretReference struct {
	Name      string
	Namespace string
}

// AuthStateCookie carries the state cookie configuration.
type AuthStateCookie struct {
	Secret   string `json:"-"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
}

// AuthSession carries session and session cookie configuration.
type AuthSession struct {
	Secret   string `json:"-"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Refresh  *bool  `json:"refresh,omitempty"`
}

// ptrBool returns a pointer to boolean.
func ptrBool(v bool) *bool {
	return &v
}
