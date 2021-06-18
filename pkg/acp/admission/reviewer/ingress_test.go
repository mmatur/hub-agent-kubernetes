package reviewer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent/pkg/acp"
	"github.com/traefik/hub-agent/pkg/acp/admission/quota"
	"github.com/traefik/hub-agent/pkg/acp/jwt"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type reviewer interface {
	CanReview(ar admv1.AdmissionReview) (bool, error)
	Review(ctx context.Context, ar admv1.AdmissionReview) ([]byte, error)
}

func ingressHandleACPName(t *testing.T, factory func(policies PolicyGetter) reviewer) {
	t.Helper()

	tests := []struct {
		desc         string
		acpName      string
		acpNamespace string
		ingNamespace string
		wantACPName  string
	}{
		{
			desc:        "Uses canonical ACP name",
			acpName:     "name@namespace",
			wantACPName: "name@namespace",
		},
		{
			desc:         "Uses ingress namespace as default namespace",
			acpName:      "name",
			ingNamespace: "namespace",
			wantACPName:  "name@namespace",
		},
		{
			desc:         "Handle empty ingress namespace",
			acpName:      "name",
			ingNamespace: "",
			wantACPName:  "name@default",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var gotACPName string
			policies := func(canonicalName string) *acp.Config {
				gotACPName = canonicalName
				return &acp.Config{JWT: &jwt.Config{}}
			}

			rev := factory(policyGetterMock(policies))
			ing := struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
			}{
				Metadata: metav1.ObjectMeta{
					Name:      "name",
					Namespace: test.ingNamespace,
					Annotations: map[string]string{
						"hub.traefik.io/access-control-policy": test.acpName,
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

			_, err = rev.Review(context.Background(), ar)
			require.NoError(t, err)
			assert.Equal(t, test.wantACPName, gotACPName)
		})
	}
}

func reviewRespectsQuotas(t *testing.T, factory func(quotas QuotaTransaction) reviewer) {
	t.Helper()

	tests := []struct {
		desc       string
		newSpec    ingressSpec
		routesLeft int
		wantErr    assert.ErrorAssertionFunc
	}{
		{
			desc: "one route left allows a new ingress with one route",
			newSpec: ingressSpec{
				Rules: []ingressRule{
					{
						HTTP: &ingressRuleHTTP{make([]interface{}, 1)},
					},
				},
			},
			routesLeft: 1,
			wantErr:    assert.NoError,
		},
		{
			desc: "one route left allows a new ingress with no HTTP rule",
			newSpec: ingressSpec{
				Rules: make([]ingressRule, 1),
			},
			routesLeft: 1,
			wantErr:    assert.NoError,
		},
		{
			desc: "no route left does not allow to add a new ingress",
			newSpec: ingressSpec{
				Rules: []ingressRule{
					{
						HTTP: &ingressRuleHTTP{make([]interface{}, 1)},
					},
				},
			},
			routesLeft: 0,
			wantErr:    assert.Error,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			quotas := quotaMock{
				txFunc: func(resourceID string, amount int) (*quota.Tx, error) {
					q := quota.New(test.routesLeft)
					return q.Tx(resourceID, amount)
				},
			}
			rev := factory(quotas)

			objBytes, err := json.Marshal(struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
				Spec     ingressSpec
			}{
				Metadata: metav1.ObjectMeta{Annotations: map[string]string{
					AnnotationHubAuth: "my-acp",
				}},
				Spec: test.newSpec,
			})
			require.NoError(t, err)

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					Object: runtime.RawExtension{
						Raw: objBytes,
					},
				},
			}

			_, err = rev.Review(context.Background(), ar)
			test.wantErr(t, err)
		})
	}
}

func reviewReleasesQuotasOnDelete(t *testing.T, factory func(quotas QuotaTransaction) reviewer) {
	t.Helper()

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

	rev := factory(quotas)

	oldIng := struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}{
		Metadata: metav1.ObjectMeta{
			Annotations: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
			},
		},
	}
	b, err := json.Marshal(oldIng)
	require.NoError(t, err)

	ar := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			Name:      "name",
			Namespace: "test",
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: b,
			},
		},
	}

	p, err := rev.Review(context.Background(), ar)
	require.NoError(t, err)

	assert.Nil(t, p)
	assert.Equal(t, "name@test", txCalledWithResourceID)
	assert.Equal(t, 0, txCalledWithAmount)
}

func reviewReleasesQuotasOnAnnotationRemove(t *testing.T, factory func(quotas QuotaTransaction) reviewer) {
	t.Helper()

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

	rev := factory(quotas)

	ing := struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}{
		Metadata: metav1.ObjectMeta{
			Annotations: map[string]string{}, // ACP annotation removed.
		},
	}
	b, err := json.Marshal(ing)
	require.NoError(t, err)

	oldIng := struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}{
		Metadata: metav1.ObjectMeta{
			Annotations: map[string]string{
				"hub.traefik.io/access-control-policy": "my-policy",
			},
		},
	}
	oldB, err := json.Marshal(oldIng)
	require.NoError(t, err)

	ar := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			Name:      "name",
			Namespace: "test",
			Object: runtime.RawExtension{
				Raw: b,
			},
			OldObject: runtime.RawExtension{
				Raw: oldB,
			},
		},
	}

	_, err = rev.Review(context.Background(), ar)
	require.NoError(t, err)

	assert.Equal(t, "name@test", txCalledWithResourceID)
	assert.Equal(t, 0, txCalledWithAmount)
}
