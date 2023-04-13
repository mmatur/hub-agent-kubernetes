/*
Copyright (C) 2022-2023 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package api

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	traefikcrdfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kinformers "k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func Test_WatcherGatewayRun(t *testing.T) {
	services := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "whoami",
				Namespace: "default",
			},
		},
	}

	namespaces := []string{"agent-ns", "default", "books"}

	tests := []struct {
		desc             string
		platformGateways []Gateway

		clusterGateways    string
		clusterAccesses    string
		clusterCollections string
		clusterAPIs        string
		clusterIngresses   string
		clusterSecrets     string
		clusterMiddlewares string

		wantGateways    string
		wantIngresses   string
		wantSecrets     string
		wantMiddlewares string
	}{
		{
			desc: "new gateway present on the platform needs to be created on the cluster",
			platformGateways: []Gateway{
				{
					Name:      "new-gateway",
					Labels:    map[string]string{"area": "stores"},
					Accesses:  []string{"products", "supply-chain"},
					Version:   "version-1",
					HubDomain: "brave-lion-123.hub-traefik.io",
					CustomDomains: []CustomDomain{
						{Name: "api.hello.example.com", Verified: true},
						{Name: "api.welcome.example.com", Verified: true},
						{Name: "not-verified.example.com", Verified: false},
					},
				},
			},
			clusterAccesses:    "testdata/new-gateway/accesses.yaml",
			clusterCollections: "testdata/new-gateway/collections.yaml",
			clusterAPIs:        "testdata/new-gateway/apis.yaml",
			wantGateways:       "testdata/new-gateway/want.gateways.yaml",
			wantIngresses:      "testdata/new-gateway/want.ingresses.yaml",
			wantSecrets:        "testdata/new-gateway/want.secrets.yaml",
			wantMiddlewares:    "testdata/new-gateway/want.middlewares.yaml",
		},
		{
			desc: "modified gateway on the platform needs to be updated on the cluster",
			platformGateways: []Gateway{
				{
					Name:      "gateway",
					Labels:    map[string]string{"area": "products", "role": "dev"},
					Accesses:  []string{"products"},
					Version:   "version-2",
					HubDomain: "brave-lion-123.hub-traefik.io",
					CustomDomains: []CustomDomain{
						{Name: "api.hello.example.com", Verified: true},
						{Name: "api.welcome.example.com", Verified: true},
						{Name: "api.new.example.com", Verified: true},
					},
				},
			},
			clusterGateways:    "testdata/update-gateway/gateways.yaml",
			clusterAccesses:    "testdata/update-gateway/accesses.yaml",
			clusterCollections: "testdata/update-gateway/collections.yaml",
			clusterAPIs:        "testdata/update-gateway/apis.yaml",
			wantGateways:       "testdata/update-gateway/want.gateways.yaml",
			wantIngresses:      "testdata/update-gateway/want.ingresses.yaml",
			wantSecrets:        "testdata/update-gateway/want.secrets.yaml",
			wantMiddlewares:    "testdata/update-gateway/want.middlewares.yaml",
		},
		{
			desc: "update change group",
			platformGateways: []Gateway{{
				Name:      "gateway",
				Labels:    map[string]string{"area": "products", "role": "dev"},
				Accesses:  []string{"products"},
				Version:   "version-2",
				HubDomain: "brave-lion-123.hub-traefik.io",
				CustomDomains: []CustomDomain{
					{Name: "api.hello.example.com", Verified: true},
					{Name: "api.welcome.example.com", Verified: true},
					{Name: "api.new.example.com", Verified: true},
				},
			}},
			clusterGateways:    "testdata/update-group/gateways.yaml",
			clusterIngresses:   "testdata/update-group/ingresses.yaml",
			clusterAccesses:    "testdata/update-group/accesses.yaml",
			clusterCollections: "testdata/update-group/collections.yaml",
			clusterAPIs:        "testdata/update-group/apis.yaml",
			wantGateways:       "testdata/update-group/want.gateways.yaml",
			wantIngresses:      "testdata/update-group/want.ingresses.yaml",
			wantSecrets:        "testdata/update-group/want.secrets.yaml",
			wantMiddlewares:    "testdata/update-group/want.middlewares.yaml",
		},
		{
			desc: "delete related ingresses when an API is removed from a gateway on the platform",
			platformGateways: []Gateway{
				{
					Name:      "modified-gateway",
					Labels:    map[string]string{"area": "products", "role": "dev"},
					Accesses:  []string{"products"},
					Version:   "version-2",
					HubDomain: "brave-lion-123.hub-traefik.io",
					CustomDomains: []CustomDomain{
						{Name: "api.hello.example.com", Verified: true},
						{Name: "api.welcome.example.com", Verified: true},
						{Name: "api.new.example.com", Verified: true},
					},
				},
			},
			clusterGateways:    "testdata/remove-api-from-gateway/gateways.yaml",
			clusterIngresses:   "testdata/remove-api-from-gateway/ingresses.yaml",
			clusterAccesses:    "testdata/remove-api-from-gateway/accesses.yaml",
			clusterCollections: "testdata/remove-api-from-gateway/collections.yaml",
			clusterAPIs:        "testdata/remove-api-from-gateway/apis.yaml",
			clusterSecrets:     "testdata/remove-api-from-gateway/secrets.yaml",
			clusterMiddlewares: "testdata/remove-api-from-gateway/middlewares.yaml",
			wantGateways:       "testdata/remove-api-from-gateway/want.gateways.yaml",
			wantIngresses:      "testdata/remove-api-from-gateway/want.ingresses.yaml",
			wantSecrets:        "testdata/remove-api-from-gateway/want.secrets.yaml",
			wantMiddlewares:    "testdata/remove-api-from-gateway/want.middlewares.yaml",
		},
		{
			desc:             "deleted gateway on the platform needs to be deleted on the cluster",
			platformGateways: []Gateway{},
			clusterGateways:  "testdata/delete-gateway/gateways.yaml",
			clusterSecrets:   "testdata/delete-gateway/secrets.yaml",
			wantSecrets:      "testdata/delete-gateway/want.secrets.yaml",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			wantGateways := loadFixtures[hubv1alpha1.APIGateway](t, test.wantGateways)
			wantIngresses := loadFixtures[netv1.Ingress](t, test.wantIngresses)
			wantSecrets := loadFixtures[corev1.Secret](t, test.wantSecrets)
			wantMiddlewares := loadFixtures[traefikv1alpha1.Middleware](t, test.wantMiddlewares)

			clusterGateways := loadFixtures[hubv1alpha1.APIGateway](t, test.clusterGateways)
			clusterAccesses := loadFixtures[hubv1alpha1.APIAccess](t, test.clusterAccesses)
			clusterCollections := loadFixtures[hubv1alpha1.APICollection](t, test.clusterCollections)
			clusterAPIs := loadFixtures[hubv1alpha1.API](t, test.clusterAPIs)
			clusterIngresses := loadFixtures[netv1.Ingress](t, test.clusterIngresses)
			clusterSecrets := loadFixtures[corev1.Secret](t, test.clusterSecrets)
			clusterMiddlewares := loadFixtures[traefikv1alpha1.Middleware](t, test.clusterMiddlewares)

			var kubeObjects []runtime.Object
			kubeObjects = append(kubeObjects, services...)
			for _, clusterIngress := range clusterIngresses {
				kubeObjects = append(kubeObjects, clusterIngress.DeepCopy())
			}
			for _, secret := range clusterSecrets {
				kubeObjects = append(kubeObjects, secret.DeepCopy())
			}

			var hubObjects []runtime.Object
			for _, clusterGateway := range clusterGateways {
				hubObjects = append(hubObjects, clusterGateway.DeepCopy())
			}
			for _, clusterAccess := range clusterAccesses {
				hubObjects = append(hubObjects, clusterAccess.DeepCopy())
			}
			for _, clusterCollection := range clusterCollections {
				hubObjects = append(hubObjects, clusterCollection.DeepCopy())
			}
			for _, clusterAPI := range clusterAPIs {
				hubObjects = append(hubObjects, clusterAPI.DeepCopy())
			}

			var traefikObjects []runtime.Object
			for _, clusterMiddleware := range clusterMiddlewares {
				traefikObjects = append(traefikObjects, clusterMiddleware.DeepCopy())
			}

			kubeClientSet := kubefake.NewSimpleClientset(kubeObjects...)
			hubClientSet := kube.NewFakeHubClientset(hubObjects...)
			traefikClientSet := traefikcrdfake.NewSimpleClientset(traefikObjects...)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

			kubeInformer := kinformers.NewSharedInformerFactory(kubeClientSet, 0)
			kubeInformer.Networking().V1().Ingresses().Informer()

			hubInformer := hubinformers.NewSharedInformerFactory(hubClientSet, 0)
			hubInformer.Hub().V1alpha1().APIGateways().Informer()
			hubInformer.Hub().V1alpha1().APIAccesses().Informer()
			hubInformer.Hub().V1alpha1().APICollections().Informer()
			hubInformer.Hub().V1alpha1().APIs().Informer()

			hubInformer.Start(ctx.Done())
			kubeInformer.Start(ctx.Done())

			hubInformer.WaitForCacheSync(ctx.Done())
			kubeInformer.WaitForCacheSync(ctx.Done())

			client := newPlatformClientMock(t)

			getGatewaysCount := 0
			// Cancel the context on the second gateways synchronization occurred.
			client.OnGetGateways().TypedReturns(test.platformGateways, nil).Run(func(_ mock.Arguments) {
				getGatewaysCount++
				if getGatewaysCount == 2 {
					cancel()
				}
			})

			client.OnGetWildcardCertificate().
				TypedReturns(edgeingress.Certificate{
					Certificate: []byte("cert"),
					PrivateKey:  []byte("private"),
				}, nil)

			var wantSyncCertificatesCustomDomains []string
			for _, clusterGateway := range clusterGateways {
				wantSyncCertificatesCustomDomains = append(wantSyncCertificatesCustomDomains, clusterGateway.Status.CustomDomains...)
			}

			if len(wantSyncCertificatesCustomDomains) > 0 {
				client.OnGetCertificateByDomains(wantSyncCertificatesCustomDomains).
					TypedReturns(edgeingress.Certificate{
						Certificate: []byte("cert"),
						PrivateKey:  []byte("private"),
					}, nil)
			}

			var wantSyncGatewaysCustomDomains []string
			for _, platformGateway := range test.platformGateways {
				for _, customDomain := range platformGateway.CustomDomains {
					if customDomain.Verified {
						wantSyncGatewaysCustomDomains = append(wantSyncGatewaysCustomDomains, customDomain.Name)
					}
				}
			}

			if len(wantSyncGatewaysCustomDomains) > 0 {
				client.OnGetCertificateByDomains(wantSyncGatewaysCustomDomains).
					TypedReturns(edgeingress.Certificate{
						Certificate: []byte("cert"),
						PrivateKey:  []byte("private"),
					}, nil)
			}

			w := NewWatcherGateway(client, kubeClientSet, kubeInformer, hubClientSet, hubInformer, traefikClientSet.TraefikV1alpha1(), &WatcherGatewayConfig{
				IngressClassName:        "ingress-class",
				AgentNamespace:          "agent-ns",
				TraefikAPIEntryPoint:    "api-entrypoint",
				TraefikTunnelEntryPoint: "tunnel-entrypoint",
				GatewaySyncInterval:     time.Millisecond,
				// we don't want to test certSync here.
				CertSyncInterval:  10 * time.Second,
				CertRetryInterval: time.Millisecond,
			})

			stop := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(stop)
			}()

			select {
			case <-ctx.Done():
				t.Log(ctx.Err())
			case <-stop:
			}

			assertGatewaysMatches(t, hubClientSet, wantGateways)
			assertSecretsMatches(t, kubeClientSet, namespaces, wantSecrets)
			assertIngressesMatches(t, kubeClientSet, namespaces, wantIngresses)
			assertMiddlewaresMatches(t, traefikClientSet, namespaces, wantMiddlewares)
		})
	}
}

func assertGatewaysMatches(t *testing.T, hubClientSet *hubfake.Clientset, want []hubv1alpha1.APIGateway) {
	t.Helper()

	slices.SortFunc(want, func(x, y hubv1alpha1.APIGateway) bool {
		return x.Name > y.Name
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	gatewayList, err := hubClientSet.HubV1alpha1().APIGateways().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var gateways []hubv1alpha1.APIGateway
	for _, gateway := range gatewayList.Items {
		gateway.Status.SyncedAt = metav1.Time{}

		gateways = append(gateways, gateway)
	}

	slices.SortFunc(gateways, func(x, y hubv1alpha1.APIGateway) bool {
		return x.Name > y.Name
	})

	assert.Equal(t, want, gateways)
}

func assertSecretsMatches(t *testing.T, kubeClientSet *kubefake.Clientset, namespaces []string, want []corev1.Secret) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	var secrets []corev1.Secret
	for _, namespace := range namespaces {
		namespaceSecretList, err := kubeClientSet.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
		require.NoError(t, err)

		secrets = append(secrets, namespaceSecretList.Items...)
	}

	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Name < secrets[j].Name
	})

	assert.Equal(t, want, secrets)
}

func assertIngressesMatches(t *testing.T, kubeClientSet *kubefake.Clientset, namespaces []string, want []netv1.Ingress) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	var ingresses []netv1.Ingress
	for _, namespace := range namespaces {
		namespaceIngressList, err := kubeClientSet.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		require.NoError(t, err)

		for _, ingress := range namespaceIngressList.Items {
			ingress.Status = netv1.IngressStatus{}

			ingresses = append(ingresses, ingress)
		}
	}

	sort.Slice(ingresses, func(i, j int) bool {
		return ingresses[i].Name < ingresses[j].Name
	})

	assert.Equal(t, want, ingresses)
}

func assertMiddlewaresMatches(t *testing.T, traefikClientSet *traefikcrdfake.Clientset, namespaces []string, want []traefikv1alpha1.Middleware) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	var middlewares []traefikv1alpha1.Middleware
	for _, namespace := range namespaces {
		namespaceMiddlewareList, err := traefikClientSet.TraefikV1alpha1().Middlewares(namespace).List(ctx, metav1.ListOptions{})
		require.NoError(t, err)

		middlewares = append(middlewares, namespaceMiddlewareList.Items...)
	}

	sort.Slice(middlewares, func(i, j int) bool {
		return middlewares[i].Name < middlewares[j].Name
	})

	assert.Equal(t, want, middlewares)
}
