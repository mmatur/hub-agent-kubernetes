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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	kubemock "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestFetcher_GetAccessControlPolicies(t *testing.T) {
	tests := []struct {
		desc    string
		fixture string
		want    map[string]*AccessControlPolicy
	}{
		{
			desc:    "basic auth",
			fixture: "fixtures/acp/basic-auth.yml",
			want: map[string]*AccessControlPolicy{
				"my-acp": {
					Name:   "my-acp",
					Method: "basicAuth",
					BasicAuth: &AccessControlPolicyBasicAuth{
						Users:                    "user:redacted",
						StripAuthorizationHeader: true,
						ForwardUsernameHeader:    "Username",
					},
				},
			},
		},
		{
			desc:    "api key",
			fixture: "fixtures/acp/api-key.yml",
			want: map[string]*AccessControlPolicy{
				"my-acp": {
					Name:   "my-acp",
					Method: "apiKey",
					APIKey: &AccessControlPolicyAPIKey{
						Header: "Api-Key",
						Query:  "api-key",
						Keys: []AccessControlPolicyAPIKeyKey{
							{ID: "user-1", Value: "redacted"},
							{ID: "user-2", Value: "redacted"},
						},
						ForwardHeaders: map[string]string{
							"Id":    "_id",
							"Group": "group",
						},
					},
				},
			},
		},
		{
			desc:    "jwt",
			fixture: "fixtures/acp/jwt.yml",
			want: map[string]*AccessControlPolicy{
				"my-acp": {
					Name:   "my-acp",
					Method: "jwt",
					JWT: &AccessControlPolicyJWT{
						SigningSecret:              "redacted",
						SigningSecretBase64Encoded: true,
						PublicKey:                  "public-key",
						StripAuthorizationHeader:   true,
						TokenQueryKey:              "token",
						Claims:                     "Equals(`group`,`dev`)",
					},
				},
			},
		},
		{
			desc:    "oidc",
			fixture: "fixtures/acp/oidc.yml",
			want: map[string]*AccessControlPolicy{
				"my-acp": {
					Name:   "my-acp",
					Method: "oidc",
					OIDC: &AccessControlPolicyOIDC{
						Issuer:   "https://foo.com",
						ClientID: "client-id",
						Secret: &SecretReference{
							Name:      "my-secret",
							Namespace: "default",
						},
						RedirectURL: "https://foobar.com/callback",
						LogoutURL:   "https://foobar.com/logout",
						Scopes:      []string{"scope"},
						AuthParams: map[string]string{
							"hd": "example.com",
						},
						StateCookie: &AuthStateCookie{
							Path:     "/",
							Domain:   "example.com",
							SameSite: "lax",
							Secure:   true,
						},
						Claims: "Equals(`group`,`dev`)",
					},
				},
			},
		},
		{
			desc:    "oidc google",
			fixture: "fixtures/acp/oidc-google.yml",
			want: map[string]*AccessControlPolicy{
				"my-acp": {
					Name:   "my-acp",
					Method: "oidcGoogle",
					OIDCGoogle: &AccessControlPolicyOIDCGoogle{
						ClientID: "client-id",
						Secret: &SecretReference{
							Name:      "my-secret",
							Namespace: "default",
						},
						RedirectURL: "https://foobar.com/callback",
						LogoutURL:   "https://foobar.com/logout",
						AuthParams: map[string]string{
							"hd": "example.com",
						},
						StateCookie: &AuthStateCookie{
							Path:     "/",
							Domain:   "example.com",
							SameSite: "lax",
							Secure:   true,
						},
						Emails: []string{"powpow@example.com"},
					},
				},
			},
		},
	}

	err := hubv1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			objects := loadK8sObjects(t, test.fixture)

			kubeClient := kubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()
			hubClient := hubkubemock.NewSimpleClientset(objects...)

			f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
			require.NoError(t, err)

			got, err := f.getAccessControlPolicies()
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}
