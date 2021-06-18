package reviewer

import (
	"context"
	"encoding/json"
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

func TestNginxIngress_CanReviewChecksKind(t *testing.T) {
	i := ingressClassesMock{
		getDefaultControllerFunc: func() (string, error) {
			return ingclass.ControllerTypeNginxOfficial, nil
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
			review := NewNginxIngress("", i, policyGetterMock(policies), quota.New(999))

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

func TestNginxIngress_CanReviewChecksIngressClass(t *testing.T) {
	tests := []struct {
		desc                   string
		annotation             string
		spec                   string
		wrongDefaultController bool
		canReview              bool
	}{
		{
			desc:      "can review a valid resource",
			canReview: true,
		},
		{
			desc:                   "can't review if the default controller is not of the correct type",
			wrongDefaultController: true,
			canReview:              false,
		},
		{
			desc:       "can review if annotation is correct",
			annotation: "nginx",
			canReview:  true,
		},
		{
			desc:       "can review if using a custom ingress class (annotation)",
			annotation: "custom-nginx-ingress-class",
			canReview:  true,
		},
		{
			desc:       "can't review if using another annotation",
			annotation: "traefik",
			canReview:  false,
		},
		{
			desc:      "can review if using a custom ingress class (spec)",
			spec:      "custom-nginx-ingress-class",
			canReview: true,
		},
		{
			desc:      "can review if using a custom ingress class with nginx community value (spec)",
			spec:      "custom-nginx-community-ingress-class",
			canReview: true,
		},
		{
			desc:      "can't review if using another controller",
			spec:      "nginx",
			canReview: false,
		},
		{
			desc:       "spec takes priority over ingAnnotation#1",
			annotation: "nginx",
			spec:       "custom-nginx-ingress-class",
			canReview:  true,
		},
		{
			desc:       "spec takes priority over ingAnnotation#2",
			annotation: "nginx",
			spec:       "nginx",
			canReview:  false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			i := ingressClassesMock{
				getControllerFunc: func(name string) string {
					if name == "custom-nginx-ingress-class" {
						return ingclass.ControllerTypeNginxOfficial
					}
					if name == "custom-nginx-community-ingress-class" {
						return ingclass.ControllerTypeNginxCommunity
					}
					return "nope"
				},
				getDefaultControllerFunc: func() (string, error) {
					if test.wrongDefaultController {
						return "nope", nil
					}
					return ingclass.ControllerTypeNginxOfficial, nil
				},
			}

			policies := func(canonicalName string) *acp.Config {
				return nil
			}
			review := NewNginxIngress("", i, policyGetterMock(policies), quota.New(999))

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
			require.NoError(t, err)
			assert.Equal(t, test.canReview, ok)
		})
	}
}

func TestNginxIngress_HandleACPName(t *testing.T) {
	factory := func(policies PolicyGetter) reviewer {
		return NewNginxIngress("", ingressClassesMock{}, policies, quota.New(999))
	}

	ingressHandleACPName(t, factory)
}

func TestNginxIngress_Review(t *testing.T) {
	tests := []struct {
		desc            string
		config          acp.Config
		prevAnnotations map[string]string
		ingAnnotations  map[string]string
		wantPatch       map[string]string
		noPatch         bool
	}{
		{
			desc: "adds authentication if ACP annotation is set",
			config: acp.Config{
				JWT: &jwt.Config{
					ForwardHeaders: map[string]string{
						"X-Header": "claimsToForward",
					},
				},
			},
			ingAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy@test",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy@test",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_X_Header; proxy_set_header X-Header $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_X_Header; proxy_set_header X-Header $value_0;\n##hub-snippet-end",
				"custom-annotation":                                 "foobar",
			},
		},
		{
			desc: "adds authentication and strip Authorization header",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			ingAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy@test",
				"custom-annotation":                    "foobar",
			},
			wantPatch: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy@test",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"custom-annotation":                                 "foobar",
			},
		},
		{
			desc: "removes authentication if ACP annotation is removed",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			prevAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy@test",
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;auth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
			},
			ingAnnotations: map[string]string{
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;auth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
			},
			wantPatch: map[string]string{
				"custom-annotation": "foobar",
			},
		},
		{
			desc: "returns no patch if annotations are already correct",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			ingAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy",
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end",
			},
			noPatch: true,
		},
		{
			desc: "preserves previous snippet annotations",
			config: acp.Config{
				JWT: &jwt.Config{
					SigningSecret: "secret",
				},
			},
			ingAnnotations: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.ingress.kubernetes.io/configuration-snippet": "# Stuff before.",
				"nginx.org/location-snippets":                       "# Stuff before.",
				"nginx.org/server-snippets":                         "# Stuff before.",
			},
			wantPatch: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "# Stuff before.\n##hub-snippet-start\nauth_request /auth;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "# Stuff before.",
			},
		},
		{
			desc: "patches between existing snippets",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: false,
				},
			},
			ingAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy",
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-bad-policy@test;}\n##hub-snippet-end\n# Stuff after.",
				"nginx.org/location-snippets":                       "# Stuff before.\n##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end\n# Stuff after.",
			},
			wantPatch: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end\n# Stuff after.",
				"nginx.org/location-snippets":                       "# Stuff before.\n##hub-snippet-start\nauth_request /auth;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "\n# Stuff after.",
			},
		},
		{
			desc: "removes hub authentication with custom snippets present",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: true,
				},
			},
			prevAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy@test",
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;auth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
			},
			ingAnnotations: map[string]string{
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;auth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization;proxy_set_header Authorization $value_0;\n##hub-snippet-end",
			},
			wantPatch: map[string]string{
				"custom-annotation":         "foobar",
				"nginx.org/server-snippets": "# Stuff before.\n",
			},
		},
		{
			desc: "adds basic authentication with username and strip authorization",
			config: acp.Config{
				BasicAuth: &basicauth.Config{
					Users:                    []string{"user:password"},
					StripAuthorizationHeader: true,
					ForwardUsernameHeader:    "User",
				},
			},
			ingAnnotations: map[string]string{
				"custom-annotation":                    "foobar",
				"hub.traefik.io/access-control-policy": "my-policy",
			},
			wantPatch: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_User; proxy_set_header User $value_0;\nauth_request_set $value_1 $upstream_http_Authorization; proxy_set_header Authorization $value_1;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_User; proxy_set_header User $value_0;\nauth_request_set $value_1 $upstream_http_Authorization; proxy_set_header Authorization $value_1;\n##hub-snippet-end",
			},
		},
		{
			desc: "adds basic authentication with username and strip authorization",
			config: acp.Config{
				DigestAuth: &digestauth.Config{
					Users:                    []string{"user:password"},
					StripAuthorizationHeader: true,
					ForwardUsernameHeader:    "User",
				},
			},
			ingAnnotations: map[string]string{
				"custom-annotation":                    "foobar",
				"hub.traefik.io/access-control-policy": "my-policy",
			},
			wantPatch: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.org/server-snippets":                         "##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_User; proxy_set_header User $value_0;\nauth_request_set $value_1 $upstream_http_Authorization; proxy_set_header Authorization $value_1;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_User; proxy_set_header User $value_0;\nauth_request_set $value_1 $upstream_http_Authorization; proxy_set_header Authorization $value_1;\n##hub-snippet-end",
			},
		},
		{
			desc: "preserves previous snippet annotations",
			config: acp.Config{
				JWT: &jwt.Config{
					SigningSecret: "secret",
				},
			},
			ingAnnotations: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.ingress.kubernetes.io/configuration-snippet": "# Stuff before.",
				"nginx.org/location-snippets":                       "# Stuff before.",
				"nginx.org/server-snippets":                         "# Stuff before.",
			},
			wantPatch: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end",
				"nginx.org/location-snippets":                       "# Stuff before.\n##hub-snippet-start\nauth_request /auth;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "# Stuff before.",
			},
		},
		{
			desc: "patches between existing snippets",
			config: acp.Config{
				JWT: &jwt.Config{
					StripAuthorizationHeader: false,
				},
			},
			ingAnnotations: map[string]string{
				"hub.traefik.io/access-control-policy":              "my-policy",
				"custom-annotation":                                 "foobar",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-bad-policy@test;}\n##hub-snippet-end\n# Stuff after.",
				"nginx.org/location-snippets":                       "# Stuff before.\n##hub-snippet-start\nauth_request /auth;\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "##hub-snippet-start\nauth_request_set $value_0 $upstream_http_Authorization; proxy_set_header Authorization $value_0;\n##hub-snippet-end\n# Stuff after.",
			},
			wantPatch: map[string]string{
				"custom-annotation":                                 "foobar",
				"hub.traefik.io/access-control-policy":              "my-policy",
				"nginx.org/server-snippets":                         "# Stuff before.\n##hub-snippet-start\nlocation /auth {proxy_pass http://hub-agent.default.svc.cluster.local/my-policy@test;}\n##hub-snippet-end\n# Stuff after.",
				"nginx.org/location-snippets":                       "# Stuff before.\n##hub-snippet-start\nauth_request /auth;\n##hub-snippet-end",
				"nginx.ingress.kubernetes.io/auth-url":              "http://hub-agent.default.svc.cluster.local/my-policy@test",
				"nginx.ingress.kubernetes.io/configuration-snippet": "\n# Stuff after.",
			},
		},
		{
			desc:    "no previous ACP and no current ACP returns an empty patch",
			noPatch: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			policies := func(canonicalName string) *acp.Config {
				return &test.config
			}
			rev := NewNginxIngress("http://hub-agent.default.svc.cluster.local", ingressClassesMock{}, policyGetterMock(policies), quota.New(999))

			ing := struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
			}{
				Metadata: metav1.ObjectMeta{
					Name:        "name",
					Namespace:   "test",
					Annotations: test.ingAnnotations,
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
					Annotations: test.prevAnnotations,
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

			p, err := rev.Review(context.Background(), ar)
			require.NoError(t, err)

			if test.noPatch {
				assert.Nil(t, p)
				return
			}
			assert.NotNil(t, p)

			var patches []map[string]interface{}
			err = json.Unmarshal(p, &patches)
			require.NoError(t, err)

			assert.Equal(t, 1, len(patches))
			assert.Equal(t, "replace", patches[0]["op"])
			assert.Equal(t, "/metadata/annotations", patches[0]["path"])
			assert.Equal(t, len(test.wantPatch), len(patches[0]["value"].(map[string]interface{})))
			for k := range test.wantPatch {
				assert.Equal(t, test.wantPatch[k], patches[0]["value"].(map[string]interface{})[k])
			}
		})
	}
}

func TestNginxIngress_ReviewRespectsQuotas(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		return NewNginxIngress("", ingressClassesMock{}, policies, quotas)
	}

	reviewRespectsQuotas(t, factory)
}

func TestNginxIngress_ReviewReleasesQuotasOnDelete(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		return NewNginxIngress("", ingressClassesMock{}, policies, quotas)
	}

	reviewReleasesQuotasOnDelete(t, factory)
}

func TestNginxIngress_ReviewReleasesQuotasOnAnnotationRemove(t *testing.T) {
	factory := func(quotas QuotaTransaction) reviewer {
		policies := policyGetterMock(func(string) *acp.Config {
			return &acp.Config{JWT: &jwt.Config{}}
		})
		return NewNginxIngress("", ingressClassesMock{}, policies, quotas)
	}

	reviewReleasesQuotasOnAnnotationRemove(t, factory)
}
