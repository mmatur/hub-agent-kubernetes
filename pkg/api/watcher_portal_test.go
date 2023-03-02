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
	"bytes"
	"context"
	"os"
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
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/informers"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func Test_WatcherRun(t *testing.T) {
	services := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "whoami-2",
				Namespace: "default",
				Annotations: map[string]string{
					"hub.traefik.io/openapi-path": "/spec.json",
					"hub.traefik.io/openapi-port": "8080",
				},
			},
		},
	}

	namespaces := []string{"agent-ns", "default", "my-ns"}

	tests := []struct {
		desc           string
		platformPortal Portal

		clusterPortals     string
		clusterIngresses   string
		clusterSecrets     string
		clusterMiddlewares string

		wantPortals       string
		wantIngresses     string
		wantEdgeIngresses string
		wantSecrets       string
		wantMiddlewares   string
	}{
		{
			desc: "new portal present on the platform needs to be created on the cluster",
			platformPortal: Portal{
				Name:    "new-portal",
				Version: "version-1",
				CustomDomains: []string{
					"hello.example.com",
					"welcome.example.com",
				},
			},
			wantPortals:       "testdata/new-portal/want.portals.yaml",
			wantEdgeIngresses: "testdata/new-portal/want.edge-ingresses.yaml",
			wantSecrets:       "testdata/new-portal/want.secrets.yaml",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			wantPortals := loadFixtures[hubv1alpha1.APIPortal](t, test.wantPortals)
			wantIngresses := loadFixtures[netv1.Ingress](t, test.wantIngresses)
			wantEdgeIngresses := loadFixtures[hubv1alpha1.EdgeIngress](t, test.wantEdgeIngresses)
			wantSecrets := loadFixtures[corev1.Secret](t, test.wantSecrets)
			wantMiddlewares := loadFixtures[traefikv1alpha1.Middleware](t, test.wantMiddlewares)

			clusterPortals := loadFixtures[hubv1alpha1.APIPortal](t, test.clusterPortals)
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
			for _, clusterPortal := range clusterPortals {
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

			hubInformer.Hub().V1alpha1().APIPortals().Informer()
			kubeInformer.Networking().V1().Ingresses().Informer()

			hubInformer.Start(ctx.Done())
			hubInformer.WaitForCacheSync(ctx.Done())

			kubeInformer.Start(ctx.Done())
			kubeInformer.WaitForCacheSync(ctx.Done())

			client := newPlatformClientMock(t)

			getPortalCount := 0
			// Cancel the context on the second portal synchronization occurred.
			client.OnGetPortals().TypedReturns([]Portal{test.platformPortal}, nil).Run(func(_ mock.Arguments) {
				getPortalCount++
				if getPortalCount == 2 {
					cancel()
				}
			})
			client.OnGetWildcardCertificate().
				TypedReturns(edgeingress.Certificate{
					Certificate: []byte("cert"),
					PrivateKey:  []byte("private"),
				}, nil)

			w := NewWatcherPortal(client, kubeClientSet, kubeInformer, hubClientSet, hubInformer, traefikClientSet.TraefikV1alpha1(), &WatcherConfig{
				IngressClassName:        "ingress-class",
				AgentNamespace:          "agent-ns",
				TraefikAPIEntryPoint:    "api-entrypoint",
				TraefikTunnelEntryPoint: "tunnel-entrypoint",
				DevPortalServiceName:    "dev-portal-service-name",
				DevPortalPort:           8080,
				APISyncInterval:         time.Millisecond,
				CertSyncInterval:        time.Millisecond,
				CertRetryInterval:       time.Millisecond,
			})

			stop := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(stop)
			}()

			<-stop

			assertPortalsMatches(t, hubClientSet, wantPortals)
			assertEdgeIngressesMatches(t, hubClientSet, "agent-ns", wantEdgeIngresses)
			assertSecretsMatches(t, kubeClientSet, namespaces, wantSecrets)
			assertIngressesMatches(t, kubeClientSet, namespaces, wantIngresses)
			assertMiddlewaresMatches(t, traefikClientSet, namespaces, wantMiddlewares)
		})
	}
}

func assertSecretsMatches(t *testing.T, kubeClientSet *kubemock.Clientset, namespaces []string, want []corev1.Secret) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var secrets []corev1.Secret
	for _, namespace := range namespaces {
		namespaceSecretList, err := kubeClientSet.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
		require.NoError(t, err)

		secrets = append(secrets, namespaceSecretList.Items...)
	}

	assert.ElementsMatch(t, want, secrets)
}

func assertPortalsMatches(t *testing.T, hubClientSet *hubkubemock.Clientset, want []hubv1alpha1.APIPortal) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	portalList, err := hubClientSet.HubV1alpha1().APIPortals().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var portals []hubv1alpha1.APIPortal
	for _, portal := range portalList.Items {
		portal.Status.SyncedAt = metav1.Time{}

		portals = append(portals, portal)
	}

	sort.Slice(portals, func(i, j int) bool {
		return portals[i].Name < portals[j].Name
	})

	assert.Equal(t, want, portals)
}

func assertEdgeIngressesMatches(t *testing.T, hubClientSet *hubkubemock.Clientset, namespace string, want []hubv1alpha1.EdgeIngress) {
	t.Helper()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	edgeIngresses, err := hubClientSet.HubV1alpha1().EdgeIngresses(namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var gotEdgeIngresses []hubv1alpha1.EdgeIngress
	for _, portal := range edgeIngresses.Items {
		portal.Status.SyncedAt = metav1.Time{}

		gotEdgeIngresses = append(gotEdgeIngresses, portal)
	}

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	assert.Equal(t, want, gotEdgeIngresses)
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

func loadFixtures[T any](t *testing.T, path string) []T {
	t.Helper()

	var objects []T
	if path == "" {
		return objects
	}

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(b), 1000)

	for {
		var object T
		if decoder.Decode(&object) != nil {
			break
		}

		objects = append(objects, object)
	}

	return objects
}
