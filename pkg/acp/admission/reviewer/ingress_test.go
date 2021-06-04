package reviewer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent/pkg/acp"
	"github.com/traefik/hub-agent/pkg/acp/admission"
	"github.com/traefik/hub-agent/pkg/acp/admission/reviewer"
	"github.com/traefik/hub-agent/pkg/acp/jwt"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func ingressHandleACPName(t *testing.T, factory func(policies reviewer.PolicyGetter) admission.Reviewer) {
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
