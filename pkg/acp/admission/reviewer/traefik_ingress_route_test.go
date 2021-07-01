package reviewer

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent/pkg/acp"
	"github.com/traefik/hub-agent/pkg/acp/admission/quota"
	"github.com/traefik/hub-agent/pkg/acp/basicauth"
	"github.com/traefik/hub-agent/pkg/acp/digestauth"
	"github.com/traefik/hub-agent/pkg/acp/jwt"
	traefikv1alpha1 "github.com/traefik/hub-agent/pkg/crd/api/traefik/v1alpha1"
	traefikkubemock "github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	admv1 "k8s.io/api/admission/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestTraefikIngressRoute_HandleACPName(t *testing.T) {
	factory := func(policies PolicyGetter) reviewer {
		fwdAuthMdlwrs := NewFwdAuthMiddlewares("", policies, traefikkubemock.NewSimpleClientset().TraefikV1alpha1())
		return NewTraefikIngressRoute(fwdAuthMdlwrs, quota.New(999))
	}

	ingressHandleACPName(t, factory)
}

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

			policies := func(canonicalName string) *acp.Config {
				return nil
			}
			fwdAuthMdlwrs := NewFwdAuthMiddlewares("", policyGetterMock(policies), nil)
			review := NewTraefikIngressRoute(fwdAuthMdlwrs, quota.New(999))

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
						"hub.traefik.io/access-control-policy": "my-old-policy@test",
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
									Name:      "zz-my-old-policy-test",
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
						"hub.traefik.io/access-control-policy": "my-policy@test",
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
									Name:      "zz-my-old-policy-test",
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
							Name:      "zz-my-policy-test",
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
						"hub.traefik.io/access-control-policy": "my-old-policy@test",
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
						"hub.traefik.io/access-control-policy": "my-policy@test",
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
							Name:      "zz-my-policy-test",
							Namespace: "test",
						},
					},
				},
			},
			wantAuthResponseHeaders: []string{"User", "Authorization"},
		},
		{
			desc: "add Digest authentication",
			config: &acp.Config{DigestAuth: &digestauth.Config{
				StripAuthorizationHeader: true,
				ForwardUsernameHeader:    "User",
			}},
			oldIng: traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-old-policy@test",
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
						"hub.traefik.io/access-control-policy": "my-policy@test",
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
							Name:      "zz-my-policy-test",
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

			traefikClientSet := traefikkubemock.NewSimpleClientset()
			policies := func(canonicalName string) *acp.Config {
				return test.config
			}
			fwdAuthMdlwrs := NewFwdAuthMiddlewares("", policyGetterMock(policies), traefikClientSet.TraefikV1alpha1())
			rev := NewTraefikIngressRoute(fwdAuthMdlwrs, quota.New(999))

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

			m, err := traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy-test", metav1.GetOptions{})
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
		{
			desc: "Update middleware with digest configuration",
			config: &acp.Config{
				DigestAuth: &digestauth.Config{
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
					Name:      "zz-my-policy-test",
					Namespace: "test",
				},
				Spec: traefikv1alpha1.MiddlewareSpec{
					ForwardAuth: &traefikv1alpha1.ForwardAuth{
						AuthResponseHeaders: []string{"fwdHeader"},
					},
				},
			}
			traefikClientSet := traefikkubemock.NewSimpleClientset(&middleware)
			policies := func(canonicalName string) *acp.Config {
				return test.config
			}
			fwdAuthMdlwrs := NewFwdAuthMiddlewares("", policyGetterMock(policies), traefikClientSet.TraefikV1alpha1())
			rev := NewTraefikIngressRoute(fwdAuthMdlwrs, quota.New(999))

			ing := traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "name",
					Namespace: "test",
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-policy@test",
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

			m, err := traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy-test", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.NotNil(t, m)
			assert.Equal(t, []string{"fwdHeader"}, m.Spec.ForwardAuth.AuthResponseHeaders)

			p, err := rev.Review(context.Background(), ar)
			assert.NoError(t, err)
			assert.NotNil(t, p)

			m, err = traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy-test", metav1.GetOptions{})
			assert.NoError(t, err)
			assert.NotNil(t, m)

			assert.Equal(t, test.wantAuthResponseHeaders, m.Spec.ForwardAuth.AuthResponseHeaders)
		})
	}
}

func TestTraefikIngressRoute_ReviewRespectsQuotas(t *testing.T) {
	tests := []struct {
		desc       string
		oldSpec    *traefikv1alpha1.IngressRouteSpec
		newSpec    traefikv1alpha1.IngressRouteSpec
		routesLeft int
		wantErr    assert.ErrorAssertionFunc
	}{
		{
			desc:    "one route left allows a new ingress with one route",
			oldSpec: nil,
			newSpec: traefikv1alpha1.IngressRouteSpec{
				Routes: make([]traefikv1alpha1.Route, 1),
			},
			routesLeft: 1,
			wantErr:    assert.NoError,
		},
		{
			desc:    "no route left does not allow to add a new ingress",
			oldSpec: nil,
			newSpec: traefikv1alpha1.IngressRouteSpec{
				Routes: make([]traefikv1alpha1.Route, 1),
			},
			routesLeft: 0,
			wantErr:    assert.Error,
		},
		{
			desc: "two routes left allows to update an ingress that had one route to have two routes",
			oldSpec: &traefikv1alpha1.IngressRouteSpec{
				Routes: make([]traefikv1alpha1.Route, 1),
			},
			newSpec: traefikv1alpha1.IngressRouteSpec{
				Routes: make([]traefikv1alpha1.Route, 1),
			},
			routesLeft: 1,
			wantErr:    assert.NoError,
		},
		{
			desc: "no route left does not allow to update an ingress that had one route to have two routes",
			oldSpec: &traefikv1alpha1.IngressRouteSpec{
				Routes: make([]traefikv1alpha1.Route, 1),
			},
			newSpec: traefikv1alpha1.IngressRouteSpec{
				Routes: make([]traefikv1alpha1.Route, 2),
			},
			routesLeft: 0,
			wantErr:    assert.Error,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			policies := policyGetterMock(func(string) *acp.Config {
				return &acp.Config{JWT: &jwt.Config{}}
			})
			fwdAuthMdlwrs := NewFwdAuthMiddlewares(
				"",
				policies,
				traefikkubemock.NewSimpleClientset().TraefikV1alpha1(),
			)
			quotas := quotaMock{
				txFunc: func(resourceID string, amount int) (*quota.Tx, error) {
					q := quota.New(test.routesLeft)
					return q.Tx(resourceID, amount)
				},
			}
			rev := NewTraefikIngressRoute(fwdAuthMdlwrs, quotas)

			objBytes, err := json.Marshal(traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					"hub.traefik.io/access-control-policy": "my-acp",
				}},
				Spec: test.newSpec,
			})
			require.NoError(t, err)

			var oldObjBytes []byte
			if test.oldSpec != nil {
				oldObjBytes, err = json.Marshal(traefikv1alpha1.IngressRoute{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": "my-acp",
					}},
					Spec: *test.oldSpec,
				})
				require.NoError(t, err)
			}

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: objBytes,
					},
					OldObject: runtime.RawExtension{
						Raw: oldObjBytes,
					},
				},
			}

			_, err = rev.Review(context.Background(), ar)
			test.wantErr(t, err)
		})
	}
}

func TestTraefikIngressRoute_ReviewReleasesQuotasOnDelete(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		fwdAuthMdlwrs := NewFwdAuthMiddlewares(
			"",
			policies,
			traefikkubemock.NewSimpleClientset().TraefikV1alpha1(),
		)

		return NewTraefikIngressRoute(fwdAuthMdlwrs, quotas)
	}

	reviewReleasesQuotasOnDelete(t, factory)
}

func TestTraefikIngressRoute_ReviewReleasesQuotasOnAnnotationRemove(t *testing.T) {
	policies := policyGetterMock(func(string) *acp.Config {
		return &acp.Config{JWT: &jwt.Config{}}
	})
	fwdAuthMdlwrs := NewFwdAuthMiddlewares(
		"",
		policies,
		traefikkubemock.NewSimpleClientset().TraefikV1alpha1(),
	)
	var (
		txCalledWithResourceID string
		txCalledWithAmount     int
	)
	quotas := quotaMock{
		txFunc: func(resourceID string, amount int) (*quota.Tx, error) {
			txCalledWithResourceID = resourceID
			txCalledWithAmount = amount

			// Since we can't create valid transactions from outside the quota package,
			// we just return one that is always valid.
			q := quota.New(999)
			return q.Tx(resourceID, amount)
		},
	}
	rev := NewTraefikIngressRoute(fwdAuthMdlwrs, quotas)

	objBytes, err := json.Marshal(traefikv1alpha1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
	})
	require.NoError(t, err)

	oldObjBytes, err := json.Marshal(traefikv1alpha1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			"hub.traefik.io/access-control-policy": "my-acp",
		}},
	})
	require.NoError(t, err)

	ar := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			Name:      "name",
			Namespace: "test",
			Object: runtime.RawExtension{
				Raw: objBytes,
			},
			OldObject: runtime.RawExtension{
				Raw: oldObjBytes,
			},
		},
	}

	_, err = rev.Review(context.Background(), ar)
	assert.NoError(t, err)

	assert.Equal(t, "name@test", txCalledWithResourceID)
	assert.Equal(t, 0, txCalledWithAmount)
}
