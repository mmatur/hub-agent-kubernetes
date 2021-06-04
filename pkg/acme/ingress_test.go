package acme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	hubkubemock "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestController_syncIngress(t *testing.T) {
	tests := []struct {
		desc                string
		ing                 *netv1.Ingress
		wantIssuerCallCount int
		wantIssuerCallReq   CertificateRequest
	}{
		{
			desc: "No TLS config",
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: netv1.IngressSpec{},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "TLS config with empty Hosts",
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: netv1.IngressSpec{
					TLS: []netv1.IngressTLS{
						{
							SecretName: "missing-secret",
						},
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "TLS config with empty secret name",
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: netv1.IngressSpec{
					TLS: []netv1.IngressTLS{
						{
							SecretName: "missing-secret",
						},
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "Missing secret",
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: netv1.IngressSpec{
					TLS: []netv1.IngressTLS{
						{
							Hosts:      []string{"test.localhost"},
							SecretName: "missing-secret",
						},
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
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: netv1.IngressSpec{
					TLS: []netv1.IngressTLS{
						{
							Hosts:      []string{"test.localhost"},
							SecretName: "secret",
						},
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "Existing secret with non-matching domains",
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
				Spec: netv1.IngressSpec{
					TLS: []netv1.IngressTLS{
						{
							Hosts:      []string{"test.localhost", "test2.localhost"},
							SecretName: "secret",
						},
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

			hubClient := hubkubemock.NewSimpleClientset()
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

			ctrl := newController(t, issuer, kubeClient, hubClient, traefikClient)

			ctrl.syncIngress(test.ing)

			assert.Equal(t, test.wantIssuerCallCount, issuerCallCount)
			assert.Equal(t, test.wantIssuerCallReq, issuerCallReq)
		})
	}
}
