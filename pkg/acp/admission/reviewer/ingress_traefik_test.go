package reviewer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/acp"
	"github.com/traefik/neo-agent/pkg/acp/admission"
	"github.com/traefik/neo-agent/pkg/acp/admission/ingclass"
	"github.com/traefik/neo-agent/pkg/acp/admission/reviewer"
	"github.com/traefik/neo-agent/pkg/acp/jwt"
	traefikkubemock "github.com/traefik/neo-agent/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	admv1 "k8s.io/api/admission/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestTraefikIngress_HandleACPName(t *testing.T) {
	factory := func(policies reviewer.PolicyGetter) admission.Reviewer {
		return reviewer.NewTraefikIngress("", ingressClassesMock{}, policies, traefikkubemock.NewSimpleClientset().TraefikV1alpha1())
	}

	ingressHandleACPName(t, factory)
}

func TestTraefikIngress_CanReviewChecksKind(t *testing.T) {
	i := ingressClassesMock{
		getDefaultControllerFunc: func() (string, error) {
			return ingclass.ControllerTypeTraefik, nil
		},
	}

	tests := []struct {
		desc      string
		kind      metav1.GroupVersionKind
		canReview bool
	}{
		{
			desc: "can review networking.k8s.io v1 Ingresses",
			kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
			canReview: true,
		},
		{
			desc: "can't review invalid networking.k8s.io Ingress version",
			kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "invalid",
				Kind:    "Ingress",
			},
			canReview: false,
		},
		{
			desc: "can't review invalid networking.k8s.io Ingress group",
			kind: metav1.GroupVersionKind{
				Group:   "invalid",
				Version: "v1",
				Kind:    "Ingress",
			},
			canReview: false,
		},
		{
			desc: "can't review non Ingress networking.k8s.io v1 resources",
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
			canReview: true,
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
			review := reviewer.NewTraefikIngress("", i, policyGetterMock(policies), nil)

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

func TestTraefikIngress_CanReviewChecksIngressClass(t *testing.T) {
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
			annotation: "traefik",
			canReview:  true,
		},
		{
			desc:       "can review if using a custom ingress class (annotation)",
			annotation: "custom-traefik-ingress-class",
			canReview:  true,
		},
		{
			desc:       "can't review if using another annotation",
			annotation: "nginx",
			canReview:  false,
		},
		{
			desc:      "can review if using a custom ingress class (spec)",
			spec:      "custom-traefik-ingress-class",
			canReview: true,
		},
		{
			desc:      "can't review if using another controller",
			spec:      "nginx",
			canReview: false,
		},
		{
			desc:       "spec takes priority over annotation#1",
			annotation: "nginx",
			spec:       "custom-traefik-ingress-class",
			canReview:  true,
		},
		{
			desc:       "spec takes priority over annotation#2",
			annotation: "traefik",
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
					if name == "custom-traefik-ingress-class" {
						return ingclass.ControllerTypeTraefik
					}
					return "nope"
				},
				getDefaultControllerFunc: func() (string, error) {
					if test.wrongDefaultController {
						return "nope", nil
					}
					return ingclass.ControllerTypeTraefik, nil
				},
			}

			policies := func(canonicalName string) *acp.Config {
				return nil
			}
			review := reviewer.NewTraefikIngress("", i, policyGetterMock(policies), nil)

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

func TestTraefikIngress_ReviewAddsAuthentication(t *testing.T) {
	traefikClientSet := traefikkubemock.NewSimpleClientset()
	policies := func(canonicalName string) *acp.Config {
		return &acp.Config{JWT: &jwt.Config{}}
	}
	rev := reviewer.NewTraefikIngress("", ingressClassesMock{}, policyGetterMock(policies), traefikClientSet.TraefikV1alpha1())

	oldIng := struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}{
		Metadata: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "test",
			Annotations: map[string]string{
				reviewer.AnnotationNeoAuth:                         "my-old-policy@test",
				"custom-annotation":                                "foobar",
				"traefik.ingress.kubernetes.io/router.middlewares": "test-zz-my-old-policy-test@kubernetescrd",
			},
		},
	}
	oldB, err := json.Marshal(oldIng)
	require.NoError(t, err)

	ing := struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}{
		Metadata: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "test",
			Annotations: map[string]string{
				reviewer.AnnotationNeoAuth:                         "my-policy@test",
				"custom-annotation":                                "foobar",
				"traefik.ingress.kubernetes.io/router.middlewares": "custom-middleware@kubernetescrd",
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
			OldObject: runtime.RawExtension{
				Raw: oldB,
			},
		},
	}

	// We run the test twice to check if reviewing the same resource twice is ok.
	for i := 0; i < 2; i++ {
		p, err := rev.Review(context.Background(), ar)
		require.NoError(t, err)
		assert.NotNil(t, p)

		var patches []map[string]interface{}
		err = json.Unmarshal(p, &patches)
		require.NoError(t, err)

		assert.Equal(t, 1, len(patches))
		assert.Equal(t, "replace", patches[0]["op"])
		assert.Equal(t, "/metadata/annotations", patches[0]["path"])
		assert.Equal(t, "my-policy@test", patches[0]["value"].(map[string]interface{})["neo.traefik.io/access-control-policy"])
		assert.Equal(t, "custom-middleware@kubernetescrd,test-zz-my-policy-test@kubernetescrd", patches[0]["value"].(map[string]interface{})["traefik.ingress.kubernetes.io/router.middlewares"])
		assert.Equal(t, "foobar", patches[0]["value"].(map[string]interface{})["custom-annotation"])

		m, err := traefikClientSet.TraefikV1alpha1().Middlewares("test").Get(context.Background(), "zz-my-policy-test", metav1.GetOptions{})
		require.NoError(t, err)
		assert.NotNil(t, m)
	}
}

type policyGetterMock func(canonicalName string) *acp.Config

func (m policyGetterMock) GetConfig(canonicalName string) (*acp.Config, error) {
	return m(canonicalName), nil
}

type ingressClassesMock struct {
	getControllerFunc        func(name string) string
	getDefaultControllerFunc func() (string, error)
}

func (m ingressClassesMock) GetController(name string) string {
	return m.getControllerFunc(name)
}

func (m ingressClassesMock) GetDefaultController() (string, error) {
	return m.getDefaultControllerFunc()
}
