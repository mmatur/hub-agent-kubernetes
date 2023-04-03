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

package reviewer

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	traefikcrdfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	admv1 "k8s.io/api/admission/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestTraefikIngressRoute_CanReviewChecksKind(t *testing.T) {
	tests := []struct {
		desc      string
		kind      metav1.GroupVersionKind
		canReview bool
	}{
		{
			desc: "can review traefik.containo.us v1alpha1 IngressRoute",
			kind: metav1.GroupVersionKind{
				Group:   "traefik.containo.us",
				Version: "v1alpha1",
				Kind:    "IngressRoute",
			},
			canReview: true,
		},
		{
			desc: "can't review invalid traefik.containo.us IngressRoute version",
			kind: metav1.GroupVersionKind{
				Group:   "traefik.containo.us",
				Version: "invalid",
				Kind:    "IngressRoute",
			},
			canReview: false,
		},
		{
			desc: "can't review invalid traefik.containo.us IngressRoute Ingress group",
			kind: metav1.GroupVersionKind{
				Group:   "invalid",
				Version: "v1alpha1",
				Kind:    "IngressRoute",
			},
			canReview: false,
		},
		{
			desc: "can't review non traefik.containo.us IngressRoute resources",
			kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "NetworkPolicy",
			},
			canReview: false,
		},
		{
			desc: "can review extensions v1beta1 Ingresses",
			kind: metav1.GroupVersionKind{
				Group:   "extensions",
				Version: "v1beta1",
				Kind:    "Ingress",
			},
			canReview: false,
		},
		{
			desc: "can't review invalid extensions Ingress version",
			kind: metav1.GroupVersionKind{
				Group:   "extensions",
				Version: "invalid",
				Kind:    "Ingress",
			},
			canReview: false,
		},
		{
			desc: "can't review invalid v1beta1 Ingress group",
			kind: metav1.GroupVersionKind{
				Group:   "invalid",
				Version: "v1beta1",
				Kind:    "Ingress",
			},
			canReview: false,
		},
		{
			desc: "can't review invalid extension v1beta1 resource",
			kind: metav1.GroupVersionKind{
				Group:   "extensions",
				Version: "v1beta1",
				Kind:    "Invalid",
			},
			canReview: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			fwdAuthMdlwrs := NewFwdAuthMiddlewares("", nil, nil)
			review := NewTraefikIngressRoute(fwdAuthMdlwrs)

			var ing netv1.Ingress
			b, err := json.Marshal(ing)
			require.NoError(t, err)

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					Kind: test.kind,
					Object: runtime.RawExtension{
						Raw: b,
					},
				},
			}

			ok, err := review.CanReview(ar)
			require.NoError(t, err)
			assert.Equal(t, test.canReview, ok)
		})
	}
}

func TestTraefikIngressRoute_ReviewAddsAuthentication(t *testing.T) {
	tests := []struct {
		desc                    string
		config                  *acp.Config
		oldIng                  traefikv1alpha1.IngressRoute
		ing                     traefikv1alpha1.IngressRoute
		wantPatch               []traefikv1alpha1.Route
		wantAuthResponseHeaders []string
	}{
		{
			desc: "add JWT authentication",
			config: &acp.Config{JWT: &jwt.Config{
				ForwardHeaders: map[string]string{
					"fwdHeader": "claim",
				},
			}},
			oldIng: traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-old-policy",
						"custom-annotation":                    "foobar",
					},
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{
							Middlewares: []traefikv1alpha1.MiddlewareRef{
								{
									Name:      "custom-middleware",
									Namespace: "test",
								},
								{
									Name:      "zz-my-old-policy",
									Namespace: "test",
								},
							},
						},
					},
				},
			},
			ing: traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-policy",
						"custom-annotation":                    "foobar",
					},
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{
							Match:    "match",
							Kind:     "kind",
							Priority: 2,
							Services: []traefikv1alpha1.Service{
								{
									LoadBalancerSpec: traefikv1alpha1.LoadBalancerSpec{Name: "Name", Namespace: "ns", Kind: "kind"},
								},
							},
							Middlewares: []traefikv1alpha1.MiddlewareRef{
								{
									Name:      "custom-middleware",
									Namespace: "test",
								},
								{
									Name:      "zz-my-old-policy",
									Namespace: "test",
								},
							},
						},
					},
				},
			},
			wantPatch: []traefikv1alpha1.Route{
				{
					Match:    "match",
					Kind:     "kind",
					Priority: 2,
					Services: []traefikv1alpha1.Service{
						{
							LoadBalancerSpec: traefikv1alpha1.LoadBalancerSpec{Name: "Name", Namespace: "ns", Kind: "kind"},
						},
					},
					Middlewares: []traefikv1alpha1.MiddlewareRef{
						{
							Name:      "custom-middleware",
							Namespace: "test",
						},
						{
							Name:      "zz-my-policy",
							Namespace: "test",
						},
					},
				},
			},
			wantAuthResponseHeaders: []string{"fwdHeader"},
		},
		{
			desc: "add Basic authentication",
			config: &acp.Config{BasicAuth: &basicauth.Config{
				StripAuthorizationHeader: true,
				ForwardUsernameHeader:    "User",
			}},
			oldIng: traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-old-policy",
						"custom-annotation":                    "foobar",
					},
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{{}},
				},
			},
			ing: traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-policy",
						"custom-annotation":                    "foobar",
					},
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{{}},
				},
			},
			wantPatch: []traefikv1alpha1.Route{
				{
					Middlewares: []traefikv1alpha1.MiddlewareRef{
						{
							Name:      "zz-my-policy",
							Namespace: "test",
						},
					},
				},
			},
			wantAuthResponseHeaders: []string{"User", "Authorization"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			traefikClientSet := traefikcrdfake.NewSimpleClientset()

			policies := newPolicyGetterMock(t)
			policies.OnGetConfig("my-policy").TypedReturns(test.config, nil).Once()

			fwdAuthMdlwrs := NewFwdAuthMiddlewares("", policies, traefikClientSet.TraefikV1alpha1())
			rev := NewTraefikIngressRoute(fwdAuthMdlwrs)

			oldB, err := json.Marshal(test.oldIng)
			require.NoError(t, err)

			b, err := json.Marshal(test.ing)
			require.NoError(t, err)

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: b,
					},
					OldObject: runtime.RawExtension{
						Raw: oldB,
					},
				},
			}

			patch, err := rev.Review(context.Background(), ar)
			assert.NoError(t, err)
			assert.NotNil(t, patch)

			assert.Equal(t, 3, len(patch))
			assert.Equal(t, "replace", patch["op"])
			assert.Equal(t, "/spec/routes", patch["path"])

			b, err = json.Marshal(patch["value"])
			require.NoError(t, err)

			var middlewares []traefikv1alpha1.Route
			err = json.Unmarshal(b, &middlewares)
			require.NoError(t, err)

			for i, route := range middlewares {
				if !reflect.DeepEqual(route, test.wantPatch[i]) {
					t.Fail()
				}
			}

			m, err := traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.NotNil(t, m)

			assert.Equal(t, test.wantAuthResponseHeaders, m.Spec.ForwardAuth.AuthResponseHeaders)
		})
	}
}

func TestTraefikIngressRoute_ReviewUpdatesExistingMiddleware(t *testing.T) {
	tests := []struct {
		desc                    string
		config                  *acp.Config
		wantAuthResponseHeaders []string
	}{
		{
			desc: "Update middleware with JWT configuration",
			config: &acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			wantAuthResponseHeaders: []string{"Authorization"},
		},
		{
			desc: "Update middleware with basic configuration",
			config: &acp.Config{
				BasicAuth: &basicauth.Config{
					StripAuthorizationHeader: true,
				},
			},
			wantAuthResponseHeaders: []string{"Authorization"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			middleware := traefikv1alpha1.Middleware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zz-my-policy",
					Namespace: "test",
				},
				Spec: traefikv1alpha1.MiddlewareSpec{
					ForwardAuth: &traefikv1alpha1.ForwardAuth{
						AuthResponseHeaders: []string{"fwdHeader"},
					},
				},
			}
			traefikClientSet := traefikcrdfake.NewSimpleClientset(&middleware)

			policies := newPolicyGetterMock(t)
			policies.OnGetConfig("my-policy").TypedReturns(test.config, nil).Once()

			fwdAuthMdlwrs := NewFwdAuthMiddlewares("", policies, traefikClientSet.TraefikV1alpha1())
			rev := NewTraefikIngressRoute(fwdAuthMdlwrs)

			ing := traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-policy",
						"custom-annotation":                    "foobar",
					},
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{
							Match:    "match",
							Kind:     "kind",
							Priority: 2,
							Services: []traefikv1alpha1.Service{
								{
									LoadBalancerSpec: traefikv1alpha1.LoadBalancerSpec{Name: "Name", Namespace: "ns", Kind: "kind"},
								},
							},
							Middlewares: nil,
						},
					},
				},
			}
			b, err := json.Marshal(ing)
			require.NoError(t, err)

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: b,
					},
				},
			}

			m, err := traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.NotNil(t, m)
			assert.Equal(t, []string{"fwdHeader"}, m.Spec.ForwardAuth.AuthResponseHeaders)

			p, err := rev.Review(context.Background(), ar)
			assert.NoError(t, err)
			assert.NotNil(t, p)

			m, err = traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.NotNil(t, m)

			assert.Equal(t, test.wantAuthResponseHeaders, m.Spec.ForwardAuth.AuthResponseHeaders)
		})
	}
}
