package acme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestController_syncIngressRoute(t *testing.T) {
	tests := []struct {
		desc                string
		ingRoute            *traefikv1alpha1.IngressRoute
		wantIssuerCallCount int
		wantIssuerCallReq   CertificateRequest
	}{
		{
			desc: "No TLS config",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`)"},
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "TLS config with no domain",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Path(`/`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "secret",
						Domains:    []traefikv1alpha1.Domain{},
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "TLS config with no domain fallbacks to routes",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "missing-secret",
						Domains:    []traefikv1alpha1.Domain{},
					},
				},
			},
			wantIssuerCallCount: 1,
			wantIssuerCallReq: CertificateRequest{
				Domains:    []string{"test.localhost"},
				Namespace:  "ns",
				SecretName: "missing-secret",
			},
		},
		{
			desc: "TLS config with domains",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "missing-secret",
						Domains: []traefikv1alpha1.Domain{
							{
								Main: "test.localhost2",
								SANs: []string{"toto.test.localhost2", "titi.test.localhost2"},
							},
							{
								Main: "test.localhost3",
								SANs: []string{"*.test.localhost3"},
							},
						},
					},
				},
			},
			wantIssuerCallCount: 1,
			wantIssuerCallReq: CertificateRequest{
				Domains:    []string{"test.localhost2", "toto.test.localhost2", "titi.test.localhost2", "test.localhost3", "*.test.localhost3"},
				Namespace:  "ns",
				SecretName: "missing-secret",
			},
		},
		{
			desc: "No domains parsed from routes",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Path(`/`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "secret",
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "Missing secret",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "missing-secret",
					},
				},
			},
			wantIssuerCallCount: 1,
			wantIssuerCallReq: CertificateRequest{
				Domains:    []string{"test.localhost"},
				Namespace:  "ns",
				SecretName: "missing-secret",
			},
		},
		{
			desc: "Missing secret ignoring casing, ordering and duplicates",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test2.localhost`)"},
						{Match: "Host(`test.localhost`)"},
						{Match: "Host(`TEST2.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "missing-secret",
					},
				},
			},
			wantIssuerCallCount: 1,
			wantIssuerCallReq: CertificateRequest{
				Domains:    []string{"test.localhost", "test2.localhost"},
				Namespace:  "ns",
				SecretName: "missing-secret",
			},
		},
		{
			desc: "Existing secret with matching domains",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "secret",
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "Existing secret with matching domains ignoring casing, ordering and duplicates",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test2.localhost`)"},
						{Match: "Host(`test.localhost`)"},
						{Match: "Host(`TEST2.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "secret2",
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "Existing secret with non-matching domains",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`, `test2.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "secret",
					},
				},
			},
			wantIssuerCallCount: 1,
			wantIssuerCallReq: CertificateRequest{
				Domains:    []string{"test.localhost", "test2.localhost"},
				Namespace:  "ns",
				SecretName: "secret",
			},
		},
		{
			desc: "Existing user secret",
			ingRoute: &traefikv1alpha1.IngressRoute{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: traefikv1alpha1.IngressRouteSpec{
					Routes: []traefikv1alpha1.Route{
						{Match: "Host(`test.localhost`, `test2.localhost`)"},
					},
					TLS: &traefikv1alpha1.TLS{
						SecretName: "user-secret",
					},
				},
			},
			wantIssuerCallCount: 0,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var (
				issuerCallCount int
				issuerCallReq   CertificateRequest
			)
			issuer := func(req CertificateRequest) {
				issuerCallCount++
				issuerCallReq = req
			}

			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			kubeClient := newFakeKubeClient(t, "1.20",
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "user-secret",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "secret",
						Labels: map[string]string{
							labelManagedBy: controllerName,
						},
						Annotations: map[string]string{
							annotationCertificateDomains: "test.localhost",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "secret2",
						Labels: map[string]string{
							labelManagedBy: controllerName,
						},
						Annotations: map[string]string{
							annotationCertificateDomains: "test.localhost,test2.localhost",
						},
					},
				},
			)

			ctrl := newController(t, issuer, kubeClient, hubClient, traefikClient)

			ctrl.syncIngressRoute(test.ingRoute)

			assert.Equal(t, test.wantIssuerCallCount, issuerCallCount)
			assert.Equal(t, test.wantIssuerCallReq.Namespace, issuerCallReq.Namespace)
			assert.Equal(t, test.wantIssuerCallReq.SecretName, issuerCallReq.SecretName)
			assert.ElementsMatch(t, test.wantIssuerCallReq.Domains, issuerCallReq.Domains)
		})
	}
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
