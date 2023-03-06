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
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubemock "k8s.io/client-go/kubernetes/fake"
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

	namespaces := []string{"agent-ns", "default", "my-ns"}

	tests := []struct {
		desc             string
		platformGateways []Gateway

		clusterGateways    string
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
					Labels:    map[string]string{"area": "users"},
					Accesses:  []string{"users"},
					Version:   "version-1",
					HubDomain: "brave-lion-123.hub-traefik.io",
					CustomDomains: []CustomDomain{
						{Name: "api.hello.example.com", Verified: true},
						{Name: "api.welcome.example.com", Verified: true},
						{Name: "not-verified.example.com", Verified: false},
					},
				},
			},
			wantGateways: "testdata/new-gateway/want.gateways.yaml",
			wantSecrets:  "testdata/new-gateway/want.secrets.yaml",
		},
		{
			desc: "modified gateway on the platform needs to be updated on the cluster",
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
			clusterGateways: "testdata/update-gateway/gateways.yaml",
			clusterSecrets:  "testdata/update-gateway/secrets.yaml",
			wantGateways:    "testdata/update-gateway/want.gateways.yaml",
			wantSecrets:     "testdata/update-gateway/want.secrets.yaml",
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
			wantGateways := loadFixtures[hubv1alpha1.APIGateway](t, test.wantGateways)
			wantIngresses := loadFixtures[netv1.Ingress](t, test.wantIngresses)
			wantSecrets := loadFixtures[corev1.Secret](t, test.wantSecrets)
			wantMiddlewares := loadFixtures[traefikv1alpha1.Middleware](t, test.wantMiddlewares)

			clusterGateways := loadFixtures[hubv1alpha1.APIGateway](t, test.clusterGateways)
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
			for _, clusterPortal := range clusterGateways {
				hubObjects = append(hubObjects, clusterPortal.DeepCopy())
			}

			var traefikObjects []runtime.Object
			for _, clusterMiddleware := range clusterMiddlewares {
				traefikObjects = append(traefikObjects, clusterMiddleware.DeepCopy())
			}

			kubeClientSet := kubemock.NewSimpleClientset(kubeObjects...)
			hubClientSet := hubkubemock.NewSimpleClientset(hubObjects...)
			traefikClientSet := traefikkubemock.NewSimpleClientset(traefikObjects...)

			ctx, cancel := context.WithCancel(context.Background())

			kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 0)
			hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

			hubInformer.Hub().V1alpha1().APIGateways().Informer()
			kubeInformer.Networking().V1().Ingresses().Informer()

			hubInformer.Start(ctx.Done())
			hubInformer.WaitForCacheSync(ctx.Done())

			kubeInformer.Start(ctx.Done())
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

			var wantCustomDomains []string
			for _, platformGateway := range test.platformGateways {
				for _, customDomain := range platformGateway.CustomDomains {
					if customDomain.Verified {
						wantCustomDomains = append(wantCustomDomains, customDomain.Name)
					}
				}
			}

			if len(wantCustomDomains) > 0 {
				client.OnGetCertificateByDomains(wantCustomDomains).
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
				CertSyncInterval:        time.Millisecond,
				CertRetryInterval:       time.Millisecond,
			})

			stop := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(stop)
			}()

			<-stop

			assertGatewaysMatches(t, hubClientSet, wantGateways)
			assertSecretsMatches(t, kubeClientSet, namespaces, wantSecrets)
			assertIngressesMatches(t, kubeClientSet, namespaces, wantIngresses)
			assertMiddlewaresMatches(t, traefikClientSet, namespaces, wantMiddlewares)
		})
	}
}

func assertGatewaysMatches(t *testing.T, hubClientSet *hubkubemock.Clientset, want []hubv1alpha1.APIGateway) {
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

func assertSecretsMatches(t *testing.T, kubeClientSet *kubemock.Clientset, namespaces []string, want []corev1.Secret) {
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

func assertIngressesMatches(t *testing.T, kubeClientSet *kubemock.Clientset, namespaces []string, want []netv1.Ingress) {
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

func assertMiddlewaresMatches(t *testing.T, traefikClientSet *traefikkubemock.Clientset, namespaces []string, want []traefikv1alpha1.Middleware) {
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
