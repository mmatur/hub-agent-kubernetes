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

package oauthintro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/expr"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/token"
	"github.com/traefik/hub-agent-kubernetes/pkg/httpclient"
)

// Config configures an OAuth 2.0 Token Introspection ACP handler.
type Config struct {
	ClientConfig   ClientConfig      `json:"clientConfig,omitempty"`
	TokenSource    token.Source      `json:"tokenSource,omitempty"`
	Claims         string            `json:"claims,omitempty"`
	ForwardHeaders map[string]string `json:"forwardHeaders,omitempty"`
}

// ClientConfig configures the HTTP client of the OAuth 2.0 Token Introspection ACP handler.
type ClientConfig struct {
	httpclient.Config

	URL           string            `json:"url,omitempty"`
	Auth          ClientConfigAuth  `json:"auth,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	TokenTypeHint string            `json:"tokenTypeHint,omitempty"`
}

// ClientConfigAuth configures authentication to the Authorization Server.
type ClientConfigAuth struct {
	Kind   string          `json:"kind,omitempty"`
	Secret SecretReference `json:"secret,omitempty"`
	Key    string          `json:"-"`
	Value  string          `json:"-"`
}

// SecretReference represents a Secret Reference.
// It has enough information to retrieve secret in any namespace.
//
//nolint:musttag // TODO must be fixed
type SecretReference struct {
	Name      string
	Namespace string
}

// Handler is an OAuth 2.0 Token Introspection ACP handler.
type Handler struct {
	name string

	url           string
	headers       template.Template
	tokenTypeHint string
	httpClient    *http.Client
	auth          ClientConfigAuth

	tokenSrc             token.Source
	fwdHeaders           map[string]string
	validateCustomClaims expr.Predicate
}

// NewHandler creates a new OAuth 2.0 Token Introspection ACP Handler.
func NewHandler(cfg *Config, polName string) (*Handler, error) {
	if cfg.ClientConfig.URL == "" {
		return nil, errors.New("empty URL")
	}

	if cfg.ClientConfig.Auth.Kind == "" {
		return nil, errors.New(`empty kind: one of "Basic", "Bearer", "Header" or "Query" must be set`)
	}

	if cfg.ClientConfig.Auth.Secret.Name == "" {
		return nil, errors.New("empty secret name")
	}

	if cfg.ClientConfig.Auth.Secret.Namespace == "" {
		return nil, errors.New("empty secret namespace")
	}

	if cfg.TokenSource.Header == "" && cfg.TokenSource.Query == "" && cfg.TokenSource.Cookie == "" {
		return nil, errors.New(`at least one of "header", "query" or "cookie" must be set`)
	}

	httpClient, err := httpclient.New(cfg.ClientConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client: %w", err)
	}

	var pred expr.Predicate
	if cfg.Claims != "" {
		pred, err = expr.Parse(cfg.Claims)
		if err != nil {
			return nil, fmt.Errorf("parsing predicate: %w", err)
		}
	}

	var tmpls template.Template
	for key, val := range cfg.ClientConfig.Headers {
		if _, err = tmpls.New(key).Parse(val); err != nil {
			return nil, fmt.Errorf("parsing template for header %q: %w", key, err)
		}
	}

	return &Handler{
		name:                 polName,
		url:                  cfg.ClientConfig.URL,
		headers:              tmpls,
		tokenTypeHint:        cfg.ClientConfig.TokenTypeHint,
		httpClient:           httpClient,
		tokenSrc:             cfg.TokenSource,
		auth:                 cfg.ClientConfig.Auth,
		fwdHeaders:           cfg.ForwardHeaders,
		validateCustomClaims: pred,
	}, nil
}

// ServeHTTP serves an HTTP request.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	l := log.With().Str("handler_type", "OAuthIntro").Str("handler_name", h.name).Logger()

	tok, err := token.Extract(req, h.tokenSrc)
	if tok == "" {
		l.Debug().Err(err).Msg("No token found in request")
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	claims, err := h.introspectToken(req, tok)
	if err != nil {
		l.Error().Err(err).Msg("Unable to introspect token")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	active, ok := claims["active"].(bool)
	if !ok || !active {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if h.validateCustomClaims != nil {
		if !h.validateCustomClaims(claims) {
			rw.WriteHeader(http.StatusForbidden)
			return
		}
	}

	hdrs, err := expr.PluckClaims(h.fwdHeaders, claims)
	if err != nil {
		l.Error().Err(err).Msg("Unable to set forwarded header")
		http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	for name, vals := range hdrs {
		for _, val := range vals {
			rw.Header().Add(name, val)
		}
	}

	rw.WriteHeader(http.StatusOK)
}

func (h *Handler) introspectToken(originalReq *http.Request, tok string) (map[string]interface{}, error) {
	form := url.Values{"token": []string{tok}}
	form.Set("token", tok)
	if h.tokenTypeHint != "" {
		form.Set("token_type_hint", h.tokenTypeHint)
	}

	req, err := http.NewRequest(http.MethodPost, h.url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(originalReq.Context())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	data := struct {
		Request *http.Request
	}{
		Request: originalReq,
	}
	if len(h.headers.Templates()) > 0 {
		for _, tmpl := range h.headers.Templates() {
			var buf bytes.Buffer
			if err = tmpl.Execute(&buf, data); err != nil {
				return nil, fmt.Errorf("executing template for header %q: %w", tmpl.Name(), err)
			}
			req.Header.Set(tmpl.Name(), buf.String())
		}
	}

	switch h.auth.Kind {
	case "Query":
		q := req.URL.Query()
		q.Set(h.auth.Key, h.auth.Value)

		req.URL.RawQuery = q.Encode()

	case "Bearer", "Basic", "Header":
		req.Header.Set(h.auth.Key, h.auth.Value)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %q", resp.StatusCode)
	}

	claims := make(map[string]interface{})
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err = dec.Decode(&claims); err != nil {
		return nil, err
	}

	return claims, nil
}
