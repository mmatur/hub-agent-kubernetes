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

	jwt "github.com/dgrijalva/jwt-go"
	jwtreq "github.com/dgrijalva/jwt-go/request"
	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/acp/jwt/expr"
)

type jwtAuth struct {
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

// Config configures a JWT ACP handler.
type Config struct {
	SigningSecret              string            `json:"signing_secret"`
	SigningSecretBase64Encoded bool              `json:"signing_secret_base_64_encoded"`
	PublicKey                  string            `json:"public_key"`
	JWKsFile                   FileOrContent     `json:"jwks_file"`
	JWKsURL                    string            `json:"jwks_url"`
	StripAuthorizationHeader   bool              `json:"strip_authorization_header"`
	ForwardHeaders             map[string]string `json:"forward_headers"`
	TokenQueryKey              string            `json:"token_query_key"`
	Claims                     string            `json:"claims"`
}

// New returns a new JWT authentication middleware.
func New(cfg *Config, polName string) (http.Handler, error) {
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

	ks, err := keySet(cfg)
	if err != nil {
		return nil, err
	}

	return &jwtAuth{
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

func keySet(src *Config) (KeySet, error) {
	if src.JWKsFile != "" {
		if src.JWKsFile.IsPath() {
			return NewFileKeySet(src.JWKsFile.String()), nil
		}

		ks, err := NewContentKeySet([]byte(src.JWKsFile))
		if err != nil {
			return nil, fmt.Errorf("new content key set: %w. If using a file path, maybe the file does not exist", err)
		}
		return ks, nil
	}

	if src.JWKsURL != "" && !strings.HasPrefix(src.JWKsURL, "/") {
		return NewRemoteKeySet(src.JWKsURL), nil
	}

	return nil, nil
}

func (j *jwtAuth) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	l := log.With().Str("handler_type", "JWT").Str("handler_name", j.name).Logger()

	extractor := jwtExtractor{tokQryKey: j.tokQryKey}
	p := &jwt.Parser{UseJSONNumber: true}
	tok, err := jwtreq.ParseFromRequest(req, extractor, j.keyFunc(req.Context()), jwtreq.WithParser(p))
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

	if j.validateCustomClaims != nil {
		if !j.validateCustomClaims(tok.Claims.(jwt.MapClaims)) {
			rw.WriteHeader(http.StatusForbidden)
			return
		}
	}

	hdrs, err := expr.PluckClaims(j.fwdHeaders, tok.Claims.(jwt.MapClaims))
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

	if j.stripAuthorization {
		rw.Header().Del("Authorization")
	}

	rw.WriteHeader(http.StatusOK)
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

// keyFunc returns a function to find the correct key to validate its given JWT's signature.
func (j *jwtAuth) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(tok *jwt.Token) (key interface{}, err error) {
		var prefix string
		if len(tok.Method.Alg()) > 2 {
			prefix = tok.Method.Alg()[:2]
		}

		kid, _ := tok.Header["kid"].(string)

		switch prefix {
		case "RS", "ES":
			if kid != "" {
				return j.resolveKey(ctx, tok, kid)
			}

			if j.pubKey == nil {
				return nil, errors.New("no public key configured")
			}
			return j.pubKey, nil

		case "HS":
			if j.signingSecret == "" {
				return nil, errors.New("no signing secret configured")
			}
			return []byte(j.signingSecret), nil

		default:
			return nil, fmt.Errorf("unsupported signing algorithm %q", tok.Method.Alg())
		}
	}
}

// resolveKey finds the correct key that was used to sign the given JWT.
func (j *jwtAuth) resolveKey(ctx context.Context, tok *jwt.Token, kid string) (interface{}, error) {
	ks := j.keySet
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

		var err error
		ks, err = j.remoteKeySet(c["iss"].(string))
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
func (j *jwtAuth) remoteKeySet(iss string) (*RemoteKeySet, error) {
	base, err := url.Parse(iss)
	if err != nil {
		return nil, err
	}

	parsed, err := base.Parse(j.jwksURL)
	if err != nil {
		return nil, err
	}

	ksURL := parsed.String()

	j.dynKeySetsMu.RLock()
	rks, ok := j.dynKeySets[ksURL]
	j.dynKeySetsMu.RUnlock()
	if ok {
		return rks, nil
	}

	j.dynKeySetsMu.Lock()
	rks = j.dynKeySets[ksURL]
	if rks == nil {
		rks = NewRemoteKeySet(ksURL)
		j.dynKeySets[ksURL] = rks
	}
	j.dynKeySetsMu.Unlock()

	return rks, nil
}
