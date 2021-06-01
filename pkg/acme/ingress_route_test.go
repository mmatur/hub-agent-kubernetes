package acme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	traefikv1alpha1 "github.com/traefik/neo-agent/pkg/crd/api/traefik/v1alpha1"
	neokubemock "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/neo-agent/pkg/crd/generated/client/traefik/clientset/versioned/fake"
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

			neoClient := neokubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			kubeClient := newFakeKubeClient(t, &corev1.Secret{
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
			})

			ctrl := newController(t, issuer, kubeClient, neoClient, traefikClient)

			ctrl.syncIngressRoute(test.ingRoute)

			assert.Equal(t, test.wantIssuerCallCount, issuerCallCount)
			assert.Equal(t, test.wantIssuerCallReq, issuerCallReq)
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
			desc: "Host rule",
			rule: "Host(`foo.localhost`)",
			want: []string{"foo.localhost"},
		},
		{
			desc: "Host rule with multiple domains",
			rule: "Host(`foo.localhost`, `bar.localhost`)",
			want: []string{"foo.localhost", "bar.localhost"},
		},
		{
			desc: "Multiple Host rules",
			rule: "Host(`foo.localhost`) || Host(`bar.localhost`) || Host(`baz.localhost`)",
			want: []string{"foo.localhost", "bar.localhost", "baz.localhost"},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			domains := parseDomains(test.rule)
			assert.Equal(t, test.want, domains)
		})
	}
}
