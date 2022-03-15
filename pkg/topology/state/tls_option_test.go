package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetTLSOptions(t *testing.T) {
	kubeClient := kubemock.NewSimpleClientset()
	// Faking having Traefik CRDs installed on cluster.
	kubeClient.Resources = append(kubeClient.Resources, &metav1.APIResourceList{
		GroupVersion: traefikv1alpha1.SchemeGroupVersion.String(),
		APIResources: []metav1.APIResource{
			{
				Kind: ResourceKindIngressRoute,
			},
			{
				Kind: ResourceKindTraefikService,
			},
			{
				Kind: ResourceKindTLSOption,
			},
		},
	})

	hubClient := hubkubemock.NewSimpleClientset()
	traefikClient := traefikkubemock.NewSimpleClientset([]runtime.Object{
		&traefikv1alpha1.TLSOption{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-tls-option",
				Namespace: "myns",
			},
			Spec: traefikv1alpha1.TLSOptionSpec{
				MinVersion:   "VersionTLS12",
				MaxVersion:   "VersionTLS13",
				CipherSuites: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
				CurvePreferences: []string{
					"CurveP521",
					"CurveP384",
				},
				ClientAuth: traefikv1alpha1.ClientAuth{
					SecretNames: []string{
						"tests/clientca1.crt",
						"tests/clientca2.crt",
					},
					ClientAuthType: "RequireAndVerifyClientCert",
				},
				SniStrict:                true,
				PreferServerCipherSuites: true,
			},
		},
		&traefikv1alpha1.TLSOption{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: "myns",
			},
			Spec: traefikv1alpha1.TLSOptionSpec{
				MinVersion: "VersionTLS13",
			},
		},
	}...)

	f, err := watchAll(context.Background(), kubeClient, hubClient, traefikClient, "v1.20.1", "cluster-id")
	require.NoError(t, err)

	got, err := f.getTLSOptions()
	require.NoError(t, err)

	assert.Equal(t, map[string]*TLSOptions{
		"my-tls-option@myns": {
			Name:         "my-tls-option",
			Namespace:    "myns",
			MinVersion:   "VersionTLS12",
			MaxVersion:   "VersionTLS13",
			CipherSuites: []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
			CurvePreferences: []string{
				"CurveP521",
				"CurveP384",
			},
			ClientAuth: traefikv1alpha1.ClientAuth{
				SecretNames: []string{
					"tests/clientca1.crt",
					"tests/clientca2.crt",
				},
				ClientAuthType: "RequireAndVerifyClientCert",
			},
			SniStrict:                true,
			PreferServerCipherSuites: true,
		},
		"default@myns": {
			Name:       "default",
			Namespace:  "myns",
			MinVersion: "VersionTLS13",
		},
	}, got)
}
