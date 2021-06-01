package acme

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/neo-agent/pkg/acme/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
)

func TestManager_ObtainCertificate(t *testing.T) {
	kubeClient := newFakeKubeClient(t)
	mgr := newManager(t, nil, kubeClient)

	want := CertificateRequest{
		Domains:    []string{"test.localhost"},
		Namespace:  "ns",
		SecretName: "name",
	}
	mgr.ObtainCertificate(want)

	got, exists := mgr.reqs["name@ns"]
	require.True(t, exists)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, mgr.workqueue.Len())

	want = CertificateRequest{
		Domains:    []string{"test2.localhost"},
		Namespace:  "ns",
		SecretName: "name",
	}
	mgr.ObtainCertificate(want)

	got, exists = mgr.reqs["name@ns"]
	require.True(t, exists)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, mgr.workqueue.Len())
}

func TestManager_resolveAndStoreCertificate(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tests := []struct {
		desc                    string
		req                     CertificateRequest
		resolverErr             error
		wantResolverCallCount   int
		wantResolverCallDomains []string
	}{
		{
			desc: "Unexpected error during certificate resolving",
			req: CertificateRequest{
				Domains:    []string{"test.localhost"},
				Namespace:  "ns",
				SecretName: "name",
			},
			resolverErr:             errors.New("unexpected error"),
			wantResolverCallCount:   1,
			wantResolverCallDomains: []string{"test.localhost"},
		},
		{
			desc: "Resolve and store certificate in a secret",
			req: CertificateRequest{
				Domains:    []string{"test.localhost"},
				Namespace:  "ns",
				SecretName: "name",
			},
			wantResolverCallCount:   1,
			wantResolverCallDomains: []string{"test.localhost"},
		},
		{
			desc: "Resolve and store certificate in an existing secret",
			req: CertificateRequest{
				Domains:    []string{"test.localhost"},
				Namespace:  "ns",
				SecretName: "existing-secret",
			},
			wantResolverCallCount:   1,
			wantResolverCallDomains: []string{"test.localhost"},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var (
				resolverCallCount   int
				resolverCallDomains []string
			)
			resolver := resolverMock(func(domains []string) (client.Certificate, error) {
				resolverCallCount++
				resolverCallDomains = domains

				cert := client.Certificate{
					Certificate: []byte("cert"),
					PrivateKey:  []byte("key"),
					Domains:     domains,
					NotBefore:   now,
					NotAfter:    now.Add(24 * time.Hour),
				}
				return cert, test.resolverErr
			})

			kubeClient := newFakeKubeClient(t, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "existing-secret",
					Labels: map[string]string{
						labelManagedBy: controllerName,
					},
				},
				Data: map[string][]byte{
					"tls.crt": []byte("cert2"),
					"tls.key": []byte("key2"),
				},
			})

			mgr := newManager(t, resolver, kubeClient)

			err := mgr.resolveAndStoreCertificate(test.req)
			if test.resolverErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, test.wantResolverCallCount, resolverCallCount)
			assert.Equal(t, test.wantResolverCallDomains, resolverCallDomains)

			if test.resolverErr != nil {
				return
			}

			secret, err := kubeClient.CoreV1().Secrets(test.req.Namespace).Get(context.Background(), test.req.SecretName, metav1.GetOptions{})
			require.NoError(t, err)

			want := &corev1.Secret{
				Type: "kubernetes.io/tls",
				ObjectMeta: metav1.ObjectMeta{
					Namespace: test.req.Namespace,
					Name:      test.req.SecretName,
					Labels:    map[string]string{labelManagedBy: controllerName},
					Annotations: map[string]string{
						annotationCertificateDomains:   strings.Join(test.req.Domains, ","),
						annotationCertificateNotBefore: strconv.Itoa(int(now.UTC().Unix())),
						annotationCertificateNotAfter:  strconv.Itoa(int(now.Add(24 * time.Hour).Unix())),
					},
				},
				Data: map[string][]byte{
					"tls.crt": []byte("cert"),
					"tls.key": []byte("key"),
				},
			}
			assert.Equal(t, want, secret)
		})
	}
}

func TestManager_renewExpiringCertificates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	secrets := []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "expired",
				Labels: map[string]string{
					labelManagedBy: controllerName,
				},
				Annotations: map[string]string{
					annotationCertificateDomains:   "test.localhost",
					annotationCertificateNotBefore: strconv.Itoa(int(now.UTC().Unix())),
					annotationCertificateNotAfter:  strconv.Itoa(int(now.Add(20 * 24 * time.Hour).UTC().Unix())),
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "valid",
				Labels: map[string]string{
					labelManagedBy: controllerName,
				},
				Annotations: map[string]string{
					annotationCertificateDomains:   "test.localhost",
					annotationCertificateNotBefore: strconv.Itoa(int(now.UTC().Unix())),
					annotationCertificateNotAfter:  strconv.Itoa(int(now.Add(40 * 24 * time.Hour).UTC().Unix())),
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "user",
			},
		},
	}

	kubeClient := newFakeKubeClient(t, secrets...)
	mgr := newManager(t, nil, kubeClient)

	err := mgr.renewExpiringCertificates()
	require.NoError(t, err)

	want := CertificateRequest{
		Domains:    []string{"test.localhost"},
		Namespace:  "ns",
		SecretName: "expired",
	}
	assert.Equal(t, want, mgr.reqs["expired@ns"])

	_, exists := mgr.reqs["valid@ns"]
	assert.False(t, exists)

	_, exists = mgr.reqs["user@ns"]
	assert.False(t, exists)
}

func newManager(t *testing.T, resolver resolverMock, kubeClient clientset.Interface) *Manager {
	t.Helper()

	ctx := context.Background()
	mgr := NewManager(resolver, kubeClient)

	mgr.kubeInformers.Start(ctx.Done())
	for typ, ok := range mgr.kubeInformers.WaitForCacheSync(ctx.Done()) {
		if !ok {
			require.Fail(t, "timed out waiting for k8s object caches to sync %s", typ)
		}
	}
	return mgr
}

type resolverMock func(domains []string) (client.Certificate, error)

func (c resolverMock) Obtain(_ context.Context, domains []string) (client.Certificate, error) {
	return c(domains)
}
