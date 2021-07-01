package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent/pkg/acp"
	"github.com/traefik/hub-agent/pkg/acp/admission/ingclass"
	"github.com/traefik/hub-agent/pkg/acp/admission/quota"
	"github.com/traefik/hub-agent/pkg/acp/basicauth"
	"github.com/traefik/hub-agent/pkg/acp/digestauth"
	"github.com/traefik/hub-agent/pkg/acp/jwt"
	admv1 "k8s.io/api/admission/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestHAProxyIngress_CanReviewChecksKind(t *testing.T) {
	i := ingressClassesMock{
		getDefaultControllerFunc: func() (string, error) {
			return ingclass.ControllerTypeHAProxyCommunity, nil
		},
	}

	tests := []struct {
		desc string
		kind metav1.GroupVersionKind
		want assert.BoolAssertionFunc
	}{
		{
			desc: "can review networking.k8s.io v1 Ingresses",
			kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
			want: assert.True,
		},
		{
			desc: "can't review invalid networking.k8s.io Ingress version",
			kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "invalid",
				Kind:    "Ingress",
			},
			want: assert.False,
		},
		{
			desc: "can't review invalid networking.k8s.io Ingress group",
			kind: metav1.GroupVersionKind{
				Group:   "invalid",
				Version: "v1",
				Kind:    "Ingress",
			},
			want: assert.False,
		},
		{
			desc: "can't review non Ingress networking.k8s.io v1 resources",
			kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "NetworkPolicy",
			},
			want: assert.False,
		},
		{
			desc: "can review extensions v1beta1 Ingresses",
			kind: metav1.GroupVersionKind{
				Group:   "extensions",
				Version: "v1beta1",
				Kind:    "Ingress",
			},
			want: assert.True,
		},
		{
			desc: "can't review invalid extensions Ingress version",
			kind: metav1.GroupVersionKind{
				Group:   "extensions",
				Version: "invalid",
				Kind:    "Ingress",
			},
			want: assert.False,
		},
		{
			desc: "can't review invalid v1beta1 Ingress group",
			kind: metav1.GroupVersionKind{
				Group:   "invalid",
				Version: "v1beta1",
				Kind:    "Ingress",
			},
			want: assert.False,
		},
		{
			desc: "can't review invalid extension v1beta1 resource",
			kind: metav1.GroupVersionKind{
				Group:   "extensions",
				Version: "v1beta1",
				Kind:    "Invalid",
			},
			want: assert.False,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			policies := func(canonicalName string) *acp.Config {
				return nil
			}
			review := NewHAProxyIngress("", i, policyGetterMock(policies), quota.New(999))

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

			got, err := review.CanReview(ar)
			require.NoError(t, err)
			test.want(t, got)
		})
	}
}

func TestHAProxyIngress_CanReviewChecksIngressClass(t *testing.T) {
	tests := []struct {
		desc                   string
		annotation             string
		spec                   string
		wrongDefaultController bool
		canReview              bool
		canReviewErr           assert.ErrorAssertionFunc
	}{
		{
			desc:         "can review a valid resource",
			canReview:    true,
			canReviewErr: assert.NoError,
		},
		{
			desc:                   "can't review if the default controller is not of the correct type",
			wrongDefaultController: true,
			canReview:              false,
			canReviewErr:           assert.NoError,
		},
		{
			desc:         "can review if annotation is correct",
			annotation:   "haproxy",
			canReview:    true,
			canReviewErr: assert.NoError,
		},
		{
			desc:         "can review if using a custom ingress class (annotation)",
			annotation:   "custom-haproxy-community-ingress-class",
			canReview:    true,
			canReviewErr: assert.NoError,
		},
		{
			desc:         "can't review if using another annotation",
			annotation:   "traefik",
			canReview:    false,
			canReviewErr: assert.NoError,
		},
		{
			desc:         "can review if using a custom ingress class (spec)",
			spec:         "custom-haproxy-community-ingress-class",
			canReview:    true,
			canReviewErr: assert.NoError,
		},
		{
			desc:         "can't review if using another controller",
			spec:         "haproxy",
			canReview:    false,
			canReviewErr: assert.Error,
		},
		{
			desc:         "spec takes priority over ingAnnotation#1",
			annotation:   "haproxy",
			spec:         "custom-haproxy-community-ingress-class",
			canReview:    true,
			canReviewErr: assert.NoError,
		},
		{
			desc:         "spec takes priority over ingAnnotation#2",
			annotation:   "haproxy",
			spec:         "haproxy",
			canReview:    false,
			canReviewErr: assert.Error,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			i := ingressClassesMock{
				getControllerFunc: func(name string) (string, error) {
					if name == "custom-haproxy-community-ingress-class" {
						return ingclass.ControllerTypeHAProxyCommunity, nil
					}
					return "", errors.New("nope")
				},
				getDefaultControllerFunc: func() (string, error) {
					if test.wrongDefaultController {
						return "nope", nil
					}
					return ingclass.ControllerTypeHAProxyCommunity, nil
				},
			}

			policies := func(canonicalName string) *acp.Config {
				return nil
			}
			review := NewHAProxyIngress("", i, policyGetterMock(policies), quota.New(999))

			ing := netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": test.annotation,
					},
				},
				Spec: netv1.IngressSpec{
					IngressClassName: &test.spec,
				},
			}

			b, err := json.Marshal(ing)
			require.NoError(t, err)

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "networking.k8s.io",
						Version: "v1",
						Kind:    "Ingress",
					},
					Object: runtime.RawExtension{
						Raw: b,
					},
				},
			}

			ok, err := review.CanReview(ar)
			test.canReviewErr(t, err)
			assert.Equal(t, test.canReview, ok)
		})
	}
}

func TestHAProxyIngress_HandleACPName(t *testing.T) {
	factory := func(policies PolicyGetter) reviewer {
		return NewHAProxyIngress("", ingressClassesMock{}, policies, quota.New(999))
	}

	ingressHandleACPName(t, factory)
}

func TestHAProxyIngress_Review(t *testing.T) {
	tests := []struct {
		desc             string
		config           acp.Config
		ingAnnotation    map[string]string
		oldIngAnnotation map[string]string
		wantPatch        map[string]string
		noPatch          bool
	}{
		{
			desc: "Adds JWT authentication",
			config: acp.Config{
				JWT: &jwt.Config{
					ForwardHeaders: map[string]string{
						"X-Header": "claimsToForward",
					},
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "X-Header:req.auth_response_header.x-header",
				"custom-annotation":                    "foobar",
			},
		},
		{
			desc: "Adds authentication cross-namespace",
			config: acp.Config{
				JWT: &jwt.Config{
					ForwardHeaders: map[string]string{
						"X-Header": "claimsToForward",
					},
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy@myns",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy@myns",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@myns",
				"ingress.kubernetes.io/auth-headers":   "X-Header:req.auth_response_header.x-header",
				"custom-annotation":                    "foobar",
			},
		},
		{
			desc: "Adds authentication and strip Authorization header",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "Authorization:req.auth_response_header.authorization",
				"custom-annotation":                    "foobar",
			},
		},
		{
			desc: "Remove hub authentication",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			oldIngAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "Authorization:req.auth_response_header.authorization",
			},
			ingAnnotation: map[string]string{
				"custom-annotation":                  "foobar",
				"ingress.kubernetes.io/auth-url":     "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers": "Authorization:req.auth_response_header.authorization",
			},
			wantPatch: map[string]string{
				"custom-annotation": "foobar",
			},
		},
		{
			desc: "No patch",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			oldIngAnnotation: map[string]string{
				"custom-annotation":                  "foobar",
				"ingress.kubernetes.io/auth-url":     "http://custom",
				"ingress.kubernetes.io/auth-headers": "Custom:req.auth_response_header.custom",
			},
			ingAnnotation: map[string]string{
				"custom-annotation":                  "foobar",
				"ingress.kubernetes.io/auth-url":     "http://custom",
				"ingress.kubernetes.io/auth-headers": "Custom2:req.auth_response_header.custom2",
			},
			noPatch: true,
		},
		{
			desc: "Overrides existing external auth conf",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			ingAnnotation: map[string]string{
				"custom-annotation":                    "foobar",
				"hub.traefik.io/access-control-policy": "my-policy@test",
				"ingress.kubernetes.io/auth-url":       "http://custom",
				"ingress.kubernetes.io/auth-headers":   "Custom:req.auth_response_header.custom",
			},
			wantPatch: map[string]string{
				"custom-annotation":                    "foobar",
				"hub.traefik.io/access-control-policy": "my-policy@test",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "Authorization:req.auth_response_header.authorization",
			},
		},
		{
			desc: "Returns no patch if not there is no update to do",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy@test",
				"custom-annotation":                    "foobar",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "Authorization:req.auth_response_header.authorization",
			},
			noPatch: true,
		},
		{
			desc: "Patches between existing policies",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: false,
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "Authorization:req.auth_response_header.authorization",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
			},
		},
		{
			desc: "Adds Basic authentication",
			config: acp.Config{
				BasicAuth: &basicauth.Config{
					Users:                    []string{"user:pass"},
					Realm:                    "realm",
					StripAuthorizationHeader: true,
					ForwardUsernameHeader:    "User",
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "User:req.auth_response_header.user,Authorization:req.auth_response_header.authorization",
				"custom-annotation":                    "foobar",
			},
		},
		{
			desc: "Adds Digest authentication",
			config: acp.Config{
				DigestAuth: &digestauth.Config{
					Users:                    []string{"user:realm:pass"},
					Realm:                    "realm",
					StripAuthorizationHeader: true,
					ForwardUsernameHeader:    "User",
				},
			},
			ingAnnotation: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
				"ingress.kubernetes.io/auth-url":       "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"ingress.kubernetes.io/auth-headers":   "User:req.auth_response_header.user,Authorization:req.auth_response_header.authorization",
				"custom-annotation":                    "foobar",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			policies := func(canonicalName string) *acp.Config {
				return &test.config
			}
			rev := NewHAProxyIngress("http://hub-agent.default.svc.cluster.local", ingressClassesMock{}, policyGetterMock(policies), quota.New(999))

			ing := struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
			}{
				Metadata: metav1.ObjectMeta{
					Name:        "name",
					Namespace:   "test",
					Annotations: test.ingAnnotation,
				},
			}
			b, err := json.Marshal(ing)
			require.NoError(t, err)

			oldIng := struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
			}{
				Metadata: metav1.ObjectMeta{
					Name:        "name",
					Namespace:   "test",
					Annotations: test.oldIngAnnotation,
				},
			}
			oldB, err := json.Marshal(oldIng)
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
			require.NoError(t, err)

			if test.noPatch {
				assert.Nil(t, patch)
				return
			}
			assert.NotNil(t, patch)

			assert.Equal(t, 3, len(patch))
			assert.Equal(t, "replace", patch["op"])
			assert.Equal(t, "/metadata/annotations", patch["path"])
			assert.Equal(t, len(test.wantPatch), len(patch["value"].(map[string]string)))
			for k := range test.wantPatch {
				assert.Equal(t, test.wantPatch[k], patch["value"].(map[string]string)[k])
			}
		})
	}
}

func TestHAProxyIngress_ReviewRespectsQuotas(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		return NewHAProxyIngress("", ingressClassesMock{}, policies, quotas)
	}

	reviewRespectsQuotas(t, factory)
}

func TestHAProxyIngress_ReviewReleasesQuotasOnDelete(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		return NewHAProxyIngress("", ingressClassesMock{}, policies, quotas)
	}

	reviewReleasesQuotasOnDelete(t, factory)
}

func TestHAProxyIngress_reviewReleasesQuotasOnAnnotationRemove(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		return NewHAProxyIngress("", ingressClassesMock{}, policies, quotas)
	}

	reviewReleasesQuotasOnAnnotationRemove(t, factory)
}
