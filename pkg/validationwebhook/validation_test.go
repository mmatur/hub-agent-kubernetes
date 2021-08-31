package validationwebhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestValidationWebhook_ServeHTTP(t *testing.T) {
	tests := []struct {
		desc            string
		rawResource     string
		verifiedDomains []string
		ingressRoute    bool
		wantAllow       bool
	}{
		{
			desc:            "returns true when asking for a verified domain",
			rawResource:     `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls":[{"hosts":["domain.com"]}]}}`,
			verifiedDomains: []string{"domain.com"},
			wantAllow:       true,
		},
		{
			desc:        "returns true when ACME is disabled",
			rawResource: `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"false"}}, "spec":{"tls":[{"hosts":["unverifiedDomain.com"]}]}}`,
			wantAllow:   true,
		},
		{
			desc:        "returns true is ACME is not provided",
			rawResource: `{"spec":{"tls":[{"hosts":["unverifiedDomain.com"]}]}}`,
			wantAllow:   true,
		},
		{
			desc:        "returns false when asking for an unverified domain",
			rawResource: `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls":[{"hosts":["unverifiedDomain.com"]}]}}`,
			wantAllow:   false,
		},
		{
			desc:        "returns false when asking for at least one unverified domain",
			rawResource: `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls":[{"hosts":["domain.com", "unverifiedDomain.com"]}]}}`,
			wantAllow:   false,
		},
		{
			desc:        "returns false when asking with no domains",
			rawResource: `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls":[{"hosts":[]}]}}`,
			wantAllow:   false,
		},
		{
			desc:            "returns true when asking for a verified domain - ingressRoute",
			rawResource:     `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"Host(domain.com)"}]}}`,
			verifiedDomains: []string{"domain.com"},
			ingressRoute:    true,
			wantAllow:       true,
		},
		{
			desc:            "returns true when asking for a verified domain (TLS config) - ingressRoute",
			rawResource:     `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls": { "domains": [ {"main": "domain.com"}]}}}`,
			verifiedDomains: []string{"domain.com"},
			ingressRoute:    true,
			wantAllow:       true,
		},
		{
			desc:            "returns true when asking for a verified domain (TLS config) - ingressRoute",
			rawResource:     `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls": { "domains": [ {"main": "domain.com", "sans": ["test.domain.com"]}]}}}`,
			verifiedDomains: []string{"domain.com", "test.domain.com"},
			ingressRoute:    true,
			wantAllow:       true,
		},
		{
			desc:         "returns true when ACME is disabled - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"false"}}, "spec":{"routes":[{"match":"Host(unverifiedDomain.com)"}]}}`,
			ingressRoute: true,
			wantAllow:    true,
		},
		{
			desc:         "returns true is ACME is not provided - ingressRoute",
			rawResource:  `{"spec":{"routes":[{"match":"Host(domain.com)"}]}}`,
			ingressRoute: true,
			wantAllow:    true,
		},
		{
			desc:         "returns false when asking for an unverified domain - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"Host(unverifiedDomain.com)"}]}}`,
			ingressRoute: true,
			wantAllow:    false,
		},
		{
			desc:         "returns false when asking for at least one unverified domain - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"Host(unverifiedDomain.com) || Host(domain.com)"}]}}`,
			ingressRoute: true,
			wantAllow:    false,
		},
		{
			desc:         "returns false when asking with no domains - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"Path(/no-host)"}]}}`,
			ingressRoute: true,
			wantAllow:    false,
		},
		{
			desc:            "returns false when asking for at least one unverified domain (TLS config) - ingressRoute",
			rawResource:     `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"tls": { "domains": [ {"main": "domain.com", "sans": ["test.domain.com"]}]}}}`,
			verifiedDomains: []string{"domain.com"},
			ingressRoute:    true,
			wantAllow:       false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var kind metav1.GroupVersionKind
			if test.ingressRoute {
				kind = metav1.GroupVersionKind{
					Group:   "traefik.containo.us",
					Version: "v1alpha1",
					Kind:    "IngressRoute",
				}
			}

			ar := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					UID:  "uid",
					Kind: kind,
					Object: runtime.RawExtension{
						Raw: []byte(test.rawResource),
					},
				},
				Response: &admv1.AdmissionResponse{},
			}
			b, err := json.Marshal(ar)
			require.NoError(t, err)

			lister := domainListerMock{listVerifiedDomains: func() []string {
				return test.verifiedDomains
			}}

			h := NewHandler(lister)

			rec := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
			require.NoError(t, err)

			h.ServeHTTP(rec, req)

			var gotAr admv1.AdmissionReview
			err = json.NewDecoder(rec.Body).Decode(&gotAr)
			require.NoError(t, err)

			assert.Equal(t, "uid", string(gotAr.Response.UID))
			assert.Equal(t, test.wantAllow, gotAr.Response.Allowed)
		})
	}
}

type domainListerMock struct {
	listVerifiedDomains func() []string
}

func (d domainListerMock) ListVerifiedDomains(_ context.Context) []string {
	return d.listVerifiedDomains()
}
