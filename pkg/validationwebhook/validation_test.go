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
			rawResource:     `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"` + "Host(`domain.com`)" + `"}]}}`,
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
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"false"}}, "spec":{"routes":[{"match":"` + "Host(`unverifiedDomain.com`)" + `"}]}}`,
			ingressRoute: true,
			wantAllow:    true,
		},
		{
			desc:         "returns true is ACME is not provided - ingressRoute",
			rawResource:  `{"spec":{"routes":[{"match":"` + "Host(`domain.com`)" + `"}]}}`,
			ingressRoute: true,
			wantAllow:    true,
		},
		{
			desc:         "returns false when asking for an unverified domain - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"` + "Host(`unverifiedDomain.com`)" + `"}]}}`,
			ingressRoute: true,
			wantAllow:    false,
		},
		{
			desc:         "returns false when asking for at least one unverified domain - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"` + "Host(`unverifiedDomain.com`) || Host(`domain.com`)" + `"}]}}`,
			ingressRoute: true,
			wantAllow:    false,
		},
		{
			desc:         "returns false when asking with no domains - ingressRoute",
			rawResource:  `{"metadata":{"annotations":{"hub.traefik.io/enable-acme":"true"}}, "spec":{"routes":[{"match":"` + "Path(`/no-host`)" + `"}]}}`,
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

func Test_parseDomains(t *testing.T) {
	tests := []struct {
		desc string
		rule string
		want []string
	}{
		{
			desc: "Empty rule",
		},
		{
			desc: "No Host rule",
			rule: "Headers(`X-Forwarded-Host`, `example.com`)",
		},
		{
			desc: "Single Host rule with a single domain",
			rule: "Host(`example.com`)",
			want: []string{"example.com"},
		},
		{
			desc: "Single Host rule with a single domain: with other rule before",
			rule: "Headers(`X-Key`, `value`) && Host(`example.com`)",
			want: []string{"example.com"},
		},
		{
			desc: "Single Host rule with a single domain: with other rule after",
			rule: "Host(`example.com`) && Headers(`X-Key`, `value`)",
			want: []string{"example.com"},
		},
		{
			desc: "Multiple Host rules with a single domain",
			rule: "Host(`1.example.com`) || Host(`2.example.com`)",
			want: []string{"1.example.com", "2.example.com"},
		},
		{
			desc: "Multiple Host rules with a single domain: with other rule in between",
			rule: "Host(`1.example.com`) || Headers(`X-Key`, `value`) || Host(`2.example.com`)",
			want: []string{"1.example.com", "2.example.com"},
		},
		{
			desc: "Single Host rules with many domains",
			rule: "Host(`1.example.com`, `2.example.com`)",
			want: []string{"1.example.com", "2.example.com"},
		},
		{
			desc: "Multiple Host rules with many domains",
			rule: "Host(`1.example.com`, `2.example.com`) || Host(`3.example.com`)",
			want: []string{"1.example.com", "2.example.com", "3.example.com"},
		},
		{
			desc: "Host rule with double quotes",
			rule: `Host("example.com")`,
			want: []string{"example.com"},
		},
		{
			desc: "Invalid rule: Host with no quotes",
			rule: "Host(example.com)",
		},
		{
			desc: "Invalid rule: Host with missing starting backtick",
			rule: "Host(example.com`)",
		},
		{
			desc: "Invalid rule: Host with missing ending backtick",
			rule: "Host(`example.com)",
		},
		{
			desc: "Invalid rule: Host with missing starting double quote",
			rule: `Host(example.com")`,
		},
		{
			desc: "Invalid rule: Host with missing ending double quote",
			rule: `Host("example.com)`,
		},
		{
			desc: "Invalid rule: Host with mixed double quote and backtick",
			rule: "Host(" + `"example.com` + "`)",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			got := parseDomains(test.rule)
			assert.Equal(t, test.want, got)
		})
	}
}
