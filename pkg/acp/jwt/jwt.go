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

package jwt

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v4"
	jwtreq "github.com/golang-jwt/jwt/v4/request"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/expr"
)

// Config configures a JWT ACP handler.
type Config struct {
	SigningSecret              string
	SigningSecretBase64Encoded bool
	PublicKey                  string
	JWKsFile                   FileOrContent
	JWKsURL                    string
	StripAuthorizationHeader   bool
	ForwardHeaders             map[string]string
	TokenQueryKey              string
	Claims                     string
}

func (cfg *Config) keySet() (KeySet, error) {
	if cfg == nil {
		return nil, nil
	}

	if cfg.JWKsFile != "" {
		if cfg.JWKsFile.IsPath() {
			return NewFileKeySet(cfg.JWKsFile.String()), nil
		}

		ks, err := NewContentKeySet([]byte(cfg.JWKsFile))
		if err != nil {
			return nil, fmt.Errorf("new content key set: %w. If using a file path, maybe the file does not exist", err)
		}
		return ks, nil
	}

	if cfg.JWKsURL != "" && !strings.HasPrefix(cfg.JWKsURL, "/") {
		return NewRemoteKeySet(cfg.JWKsURL), nil
	}

	return nil, nil
}

// Handler is a JWT ACP Handler.
type Handler struct {
	name string

	signingSecret string
	pubKey        interface{}
	tokQryKey     string

	// Either `keySet` or `dynKeySets` should be set at a time.
	// If `jwksURL` is a complete URL, `keySet` is used.
	// If `jwksURL` is a path, `dynKeySets` is used.
	jwksURL      string
	keySet       KeySet
	dynKeySetsMu sync.RWMutex
	dynKeySets   map[string]*RemoteKeySet

	stripAuthorization bool
	fwdHeaders         map[string]string

	validateCustomClaims expr.Predicate
}

// NewHandler returns a new JWT ACP Handler.
func NewHandler(cfg *Config, polName string) (*Handler, error) {
	if cfg.PublicKey == "" && cfg.SigningSecret == "" && cfg.JWKsFile == "" && cfg.JWKsURL == "" {
		return nil, errors.New("at least a signing secret, public key or a JWKs file or URL is required")
	}

	var (
		pred expr.Predicate
		err  error
	)
	if cfg.Claims != "" {
		pred, err = expr.Parse(cfg.Claims)
		if err != nil {
			return nil, fmt.Errorf("make predicate: %w", err)
		}
	}

	signingSecret := cfg.SigningSecret
	if cfg.SigningSecretBase64Encoded {
		var b []byte
		b, err = base64.StdEncoding.DecodeString(signingSecret)
		if err != nil {
			return nil, fmt.Errorf("decode base64-encoded signing secret: %w", err)
		}
		signingSecret = string(b)
	}

	var pubKey interface{}
	if cfg.PublicKey != "" {
		block, _ := pem.Decode([]byte(cfg.PublicKey))
		if block == nil {
			return nil, errors.New("empty or ill-formatted public key")
		}

		pubKey, err = x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse public key: %w", err)
		}
	}

	tokenQueryKey := "jwt"
	if cfg.TokenQueryKey != "" {
		tokenQueryKey = cfg.TokenQueryKey
	}

	ks, err := cfg.keySet()
	if err != nil {
		return nil, err
	}

	return &Handler{
		name:                 polName,
		signingSecret:        signingSecret,
		pubKey:               pubKey,
		jwksURL:              cfg.JWKsURL,
		keySet:               ks,
		dynKeySets:           make(map[string]*RemoteKeySet),
		stripAuthorization:   cfg.StripAuthorizationHeader,
		fwdHeaders:           cfg.ForwardHeaders,
		tokQryKey:            tokenQueryKey,
		validateCustomClaims: pred,
	}, nil
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	l := log.With().Str("handler_type", "JWT").Str("handler_name", h.name).Logger()

	extractor := jwtExtractor{tokQryKey: h.tokQryKey}
	p := &jwt.Parser{UseJSONNumber: true}
	tok, err := jwtreq.ParseFromRequest(req, extractor, h.keyFunc(req.Context()), jwtreq.WithParser(p))
	if err != nil {
		var jwtErr *jwt.ValidationError
		if errors.As(err, &jwtErr) && jwtErr.Errors&jwt.ValidationErrorUnverifiable != 0 {
			l.Error().Err(err).Msg("Unable to verify the signing key")
		} else {
			l.Error().Err(err).Msg("Unable to parse JWT")
		}

		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if h.validateCustomClaims != nil {
		if !h.validateCustomClaims(tok.Claims.(jwt.MapClaims)) {
			rw.WriteHeader(http.StatusForbidden)
			return
		}
	}

	hdrs, err := expr.PluckClaims(h.fwdHeaders, tok.Claims.(jwt.MapClaims))
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

	if h.stripAuthorization {
		rw.Header().Add("Authorization", "")
	}

	rw.WriteHeader(http.StatusOK)
}

// keyFunc returns a function to find the correct key to validate its given JWT's signature.
func (h *Handler) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(tok *jwt.Token) (key interface{}, err error) {
		var prefix string
		if len(tok.Method.Alg()) > 2 {
			prefix = tok.Method.Alg()[:2]
		}

		kid, _ := tok.Header["kid"].(string)

		switch prefix {
		case "RS", "ES":
			if kid != "" {
				return h.resolveKey(ctx, tok, kid)
			}

			if h.pubKey == nil {
				return nil, errors.New("no public key configured")
			}
			return h.pubKey, nil

		case "HS":
			if h.signingSecret == "" {
				return nil, errors.New("no signing secret configured")
			}
			return []byte(h.signingSecret), nil

		default:
			return nil, fmt.Errorf("unsupported signing algorithm %q", tok.Method.Alg())
		}
	}
}

// resolveKey finds the correct key that was used to sign the given JWT.
func (h *Handler) resolveKey(ctx context.Context, tok *jwt.Token, kid string) (key interface{}, err error) {
	ks := h.keySet
	if ks == nil {
		c, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			return nil, errors.New("invalid JWT claims")
		}

		if _, ok = c["iss"]; !ok {
			return nil, errors.New("expected `iss` claim to be set")
		}

		if _, ok = c["iss"].(string); !ok {
			return nil, errors.New("expected `iss` claim to be a string")
		}

		ks, err = h.remoteKeySet(c["iss"].(string))
		if err != nil {
			return nil, err
		}
	}

	k, err := ks.Key(ctx, kid)
	if err != nil {
		return nil, fmt.Errorf("error searching for JSON web key: %w", err)
	}

	if k == nil {
		return nil, fmt.Errorf("no key with id %q found", kid)
	}
	return k.Key, nil
}

// remoteKeySet returns the remote key set for the given issuer, or creates a new one if none is found.
func (h *Handler) remoteKeySet(iss string) (*RemoteKeySet, error) {
	base, err := url.Parse(iss)
	if err != nil {
		return nil, err
	}

	parsed, err := base.Parse(h.jwksURL)
	if err != nil {
		return nil, err
	}

	ksURL := parsed.String()

	h.dynKeySetsMu.RLock()
	rks, ok := h.dynKeySets[ksURL]
	h.dynKeySetsMu.RUnlock()
	if ok {
		return rks, nil
	}

	h.dynKeySetsMu.Lock()
	rks = h.dynKeySets[ksURL]
	if rks == nil {
		rks = NewRemoteKeySet(ksURL)
		h.dynKeySets[ksURL] = rks
	}
	h.dynKeySetsMu.Unlock()

	return rks, nil
}

// jwtExtractor extracts JWTs from HTTP requests.
type jwtExtractor struct {
	tokQryKey string
}

// ExtractToken extracts a JWT from an HTTP request. It first looks in the "Authorization" header then in a query parameter
// named as configured by `tokQryKey`. It returns an error if no JWT was found.
func (j jwtExtractor) ExtractToken(req *http.Request) (string, error) {
	rawJWT := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if rawJWT == "" {
		rawJWT = req.URL.Query().Get(j.tokQryKey)
	}

	if rawJWT == "" {
		return "", errors.New("no JWT found in request")
	}

	return rawJWT, nil
}
