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
	"crypto/rsa"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/square/go-jose.v2"
)

const (
	validJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiZ3JwIjoiYWRtaW4ifQ.cAdgnx0BVTC53tEMQgIzP61TnoVsB3LNXhR9IYwFvgI"
	// {"sub": "1234567890", "name": "John Doe", "iat": 1516239022, "nested": {"property": "value"}}.
	validJWTWithNestedClaim = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJuZXN0ZWQiOnsicHJvcGVydHkiOiJ2YWx1ZSJ9fQ.D2wXP6ceyQebNzYtN4fm1AC5xu6IOEhQXvKvv2AXY7k"
	expiredJWT              = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiSm9obiIsImdycCI6ImFkbWluIiwiZXhwIjoxNDAwMDAwMDAwfQ.RReBcBu5AQb6kPkjY6Nm_I0Z5rPfWs35QGJIypZS0YI"
	missingGroupJWT         = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiSm9obiJ9._ffBVeLqiMIzQvXpceunEydRDsAwTWAgIGgCr5WY3ws"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		jwtCfg  Config
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "no keys",
			jwtCfg:  Config{},
			wantErr: assert.Error,
		},
		{
			name:    "signing secret",
			jwtCfg:  Config{SigningSecret: "foobar"},
			wantErr: assert.NoError,
		},
		{
			name: "base64 signing secret",
			jwtCfg: Config{
				SigningSecret: base64.StdEncoding.EncodeToString([]byte("foobar")), SigningSecretBase64Encoded: true,
			},
			wantErr: assert.NoError,
		},
		{
			name:    "invlalid base64 signing secret",
			jwtCfg:  Config{SigningSecret: "foobar", SigningSecretBase64Encoded: true},
			wantErr: assert.Error,
		},
		{
			name:    "public key",
			jwtCfg:  Config{PublicKey: validPubKey},
			wantErr: assert.NoError,
		},
		{
			name:    "invalid public key",
			jwtCfg:  Config{PublicKey: invalidPubKey},
			wantErr: assert.Error,
		},
		{
			name:    "JWK",
			jwtCfg:  Config{JWKsURL: "http://example.com"},
			wantErr: assert.NoError,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewHandler(&test.jwtCfg, "acp@my-ns")
			test.wantErr(t, err)
		})
	}
}

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		name   string
		jwtCfg Config

		token          string
		wantStatusCode int
		wantHeader     http.Header
	}{
		{
			name:           "token is missing",
			jwtCfg:         Config{SigningSecret: "bibi"},
			token:          "",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "token is valid",
			jwtCfg:         Config{SigningSecret: "bibi"},
			token:          validJWT,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "token is expired",
			jwtCfg:         Config{SigningSecret: "bibi"},
			token:          expiredJWT,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name: "token is not for required group",
			jwtCfg: Config{
				SigningSecret: "bibi",
				Claims:        "Equals(`grp`, `admin`)",
			},
			token:          missingGroupJWT,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "claims not equal",
			jwtCfg: Config{
				SigningSecret: "bibi",
				Claims:        "!Equals(`grp`, `admin`)",
			},
			token:          missingGroupJWT,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "claims not equal#2",
			jwtCfg: Config{
				SigningSecret: "bibi",
				Claims:        "!Equals(`name`, `John Doe`)",
			},
			token:          validJWTWithNestedClaim,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "group header is forwarded",
			jwtCfg: Config{
				SigningSecret:  "bibi",
				ForwardHeaders: map[string]string{"Group": "grp"},
			},
			token:          validJWT,
			wantStatusCode: http.StatusOK,
			wantHeader:     http.Header{"Group": []string{"admin"}},
		},
		{
			name:           "required `iss` property when JWKs URL is a path",
			jwtCfg:         Config{JWKsURL: "/.well-known/jwks.json"},
			token:          validJWT,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name: "nested header is forwarded (and header is canonicalized)",
			jwtCfg: Config{
				SigningSecret:  "bibi",
				ForwardHeaders: map[string]string{"nested-Property": "nested.property"},
			},
			token:          validJWTWithNestedClaim,
			wantStatusCode: http.StatusOK,
			wantHeader:     http.Header{"Nested-Property": []string{"value"}},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			middleware, err := NewHandler(&test.jwtCfg, "acp@my-ns")
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+test.token)

			middleware.ServeHTTP(rec, req)

			assert.Equal(t, test.wantStatusCode, rec.Code)

			assert.Equal(t, len(test.wantHeader), len(rec.Header()))
			for k := range test.wantHeader {
				assert.Equal(t, test.wantHeader[k], rec.Header()[k])
			}

			tokenQueryKey := "jwt"
			if test.jwtCfg.TokenQueryKey != "" {
				tokenQueryKey = test.jwtCfg.TokenQueryKey
			}
			req, err = http.NewRequest(http.MethodGet, "/?"+tokenQueryKey+"="+test.token, http.NoBody)
			require.NoError(t, err)

			middleware.ServeHTTP(rec, req)
			assert.Equal(t, test.wantStatusCode, rec.Code)
		})
	}
}

func TestExtractJWT(t *testing.T) {
	tests := []struct {
		name    string
		req     *http.Request
		wantJWT string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "JWT is found in Authorization header",
			req: &http.Request{
				Header: http.Header{
					"Authorization": []string{"Bearer J.W.T"},
				},
			},
			wantJWT: "J.W.T",
			wantErr: assert.NoError,
		},
		{
			name: "JWT is found in query parameter",
			req: &http.Request{
				URL: &url.URL{
					RawQuery: url.Values{
						"customkey": []string{"J.W.T"},
					}.Encode(),
				},
			},
			wantJWT: "J.W.T",
			wantErr: assert.NoError,
		},
		{
			name: "JWT is found nowhere",
			req: &http.Request{
				URL: &url.URL{},
			},
			wantErr: assert.Error,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			subj := jwtExtractor{
				tokQryKey: "customkey",
			}
			tok, err := subj.ExtractToken(test.req)
			test.wantErr(t, err)

			assert.Equal(t, test.wantJWT, tok)
		})
	}
}

func TestKeyFunc(t *testing.T) {
	tests := []struct {
		name    string
		handler *Handler
		tok     *jwt.Token
		wantKey interface{}
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "signing secret found",
			handler: &Handler{
				signingSecret: "signing-secret",
			},
			tok:     &jwt.Token{Method: jwt.SigningMethodHS512},
			wantKey: []byte("signing-secret"),
			wantErr: assert.NoError,
		},
		{
			name:    "no signing secret found",
			handler: &Handler{},
			tok:     &jwt.Token{Method: jwt.SigningMethodHS512},
			wantErr: assert.Error,
		},
		{
			name:    "unsupported signing algorithm",
			handler: &Handler{},
			tok:     &jwt.Token{Method: jwt.SigningMethodPS512},
			wantErr: assert.Error,
		},
		{
			name:    "no public key found",
			handler: &Handler{},
			tok:     &jwt.Token{Method: jwt.SigningMethodRS512},
			wantErr: assert.Error,
		},
		{
			name: "public key found",
			handler: &Handler{
				pubKey: rsa.PublicKey{},
			},
			tok:     &jwt.Token{Method: jwt.SigningMethodRS512},
			wantKey: rsa.PublicKey{},
			wantErr: assert.NoError,
		},
		{
			name: "jwks key found",
			handler: &Handler{
				keySet: &RemoteKeySet{
					expiry: time.Now().Add(60 * time.Second),
					keys: jose.JSONWebKeySet{
						Keys: []jose.JSONWebKey{
							{
								Key:   rsa.PublicKey{},
								KeyID: "foo",
							},
						},
					},
				},
			},
			tok:     &jwt.Token{Method: jwt.SigningMethodRS512, Header: map[string]interface{}{"kid": "foo"}},
			wantKey: rsa.PublicKey{},
			wantErr: assert.NoError,
		},
		{
			name: "jwks key not found",
			handler: &Handler{
				keySet: &RemoteKeySet{
					expiry: time.Now().Add(60 * time.Second),
					keys: jose.JSONWebKeySet{
						Keys: []jose.JSONWebKey{},
					},
				},
			},
			tok:     &jwt.Token{Method: jwt.SigningMethodRS512, Header: map[string]interface{}{"kid": "foo"}},
			wantErr: assert.Error,
		},
		{
			name:    "jwks no keyset",
			handler: &Handler{},
			tok:     &jwt.Token{Method: jwt.SigningMethodRS512, Header: map[string]interface{}{"kid": "foo"}},
			wantErr: assert.Error,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			kf := test.handler.keyFunc(context.Background())
			key, err := kf(test.tok)
			test.wantErr(t, err)

			assert.Equal(t, test.wantKey, key)
		})
	}
}

const (
	invalidPubKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA3VoPN9PKUjKFLMwOge6+
wnDi8sbETGIx2FKXGgqtAKpzmem53kRGEQg8WeqRmp12wgp74TGpkEXsGae7RS1k
enJCnma4fii+noGH7R0qKgHvPrI2Bwa9hzsH8tHxpyM3qrXslOmD45EH9SxIDUBJ
FehNdaPbLP1gFyahKMsdfxFJLUvbUycuZSJ2ZnIgeVxwm4qbSvZInL9Iu4FzuPtg
fINKcbbovy1qq4KvPIrXzhbY3PWDc6btxCf3SE0JdE1MCPThntB62/bLMSQ7xdDR
FF53oIpvxe/SCOymfWq/LW849Ytv3Xwod0+wzAP8STXG4HSELS4UedPYeHJJJYcZ
+QIDABAQ
-----END PUBLIC KEY-----
`
	validPubKey = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAnzyis1ZjfNB0bBgKFMSv
vkTtwlvBsaJq7S5wA+kzeVOVpVWwkWdVha4s38XM/pa/yr47av7+z3VTmvDRyAHc
aT92whREFpLv9cj5lTeJSibyr/Mrm/YtjCZVWgaOYIhwrXwKLqPr/11inWsAkfIy
tvHWTxZYEcXLgAXFuUuaS3uF9gEiNQwzGTU1v0FqkqTBr4B8nW3HCN47XUu0t8Y0
e+lf4s4OxQawWD79J9/5d3Ry0vbV3Am1FtGJiJvOwRsIfVChDpYStTcHTCMqtvWb
V6L11BWkpzGXSW4Hv43qa+GSYOD2QU68Mb59oSk2OB+BtOLpJofmbGEGgvmwyCI9
MwIDAQAB
-----END PUBLIC KEY-----
`
)

func Test_JWTAuthNew(t *testing.T) {
	tests := []struct {
		name    string
		static  Config
		keySet  *RemoteKeySet
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "creates new JWT handler",
			static: Config{
				SigningSecret: "bibi",
			},
			wantErr: assert.NoError,
		},
		{
			name: "creates new JWT handler with signing secret base64 encoded",
			static: Config{
				SigningSecret:              "YmliaQ==", // bibi
				SigningSecretBase64Encoded: true,
			},
			wantErr: assert.NoError,
		},
		{
			name: "creates new JWT handler",
			static: Config{
				PublicKey: validPubKey,
			},
			wantErr: assert.NoError,
		},
		{
			name:    "raises error if no signinSecret or publicKey",
			static:  Config{},
			wantErr: assert.Error,
		},
		{
			name: "raises error if invalid public key",
			static: Config{
				PublicKey: invalidPubKey,
			},
			wantErr: assert.Error,
		},
		{
			name: "raises error if claim condition is invalid",
			static: Config{
				SigningSecret: "bibi",
				Claims:        "Equals(",
			},
			wantErr: assert.Error,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewHandler(&test.static, "acp@my-ns")
			test.wantErr(t, err)
		})
	}
}
