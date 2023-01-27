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

package catalog

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
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
		desc             string
		platformCatalogs []Catalog

		clusterCatalogs  string
		clusterIngresses string
		clusterSecrets   string

		wantCatalogs      string
		wantIngresses     string
		wantEdgeIngresses string
		wantSecrets       string
	}{
		{
			desc: "new catalog present on the platform needs to be created on the cluster",
			platformCatalogs: []Catalog{
				{
					Name:          "new-catalog",
					Version:       "version-1",
					Domain:        "majestic-beaver-123.hub-traefik.io",
					CustomDomains: []string{"hello.example.com", "welcome.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
						{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
					},
				},
			},
			wantCatalogs:      "testdata/new-catalog/want.catalogs.yaml",
			wantIngresses:     "testdata/new-catalog/want.ingresses.yaml",
			wantEdgeIngresses: "testdata/new-catalog/want.edge-ingresses.yaml",
			wantSecrets:       "testdata/new-catalog/want.secrets.yaml",
		},
		{
			desc: "a catalog has been updated on the platform: last service from a namespace deleted",
			platformCatalogs: []Catalog{
				{
					Name:          "catalog",
					Version:       "version-2",
					Domain:        "majestic-beaver-123.hub-traefik.io",
					CustomDomains: []string{"hello.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 8080, OpenAPISpecURL: "http://hello.example.com/spec.json"},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
					},
				},
			},
			clusterCatalogs:   "testdata/updated-catalog-service-deleted/catalogs.yaml",
			clusterIngresses:  "testdata/updated-catalog-service-deleted/ingresses.yaml",
			clusterSecrets:    "testdata/updated-catalog-service-deleted/secrets.yaml",
			wantCatalogs:      "testdata/updated-catalog-service-deleted/want.catalogs.yaml",
			wantEdgeIngresses: "testdata/updated-catalog-service-deleted/want.edge-ingresses.yaml",
			wantIngresses:     "testdata/updated-catalog-service-deleted/want.ingresses.yaml",
			wantSecrets:       "testdata/updated-catalog-service-deleted/want.secrets.yaml",
		},
		{
			desc: "a catalog has been updated on the platform: new service in new namespace added",
			platformCatalogs: []Catalog{
				{
					Name:          "catalog",
					Version:       "version-2",
					Domain:        "majestic-beaver-123.hub-traefik.io",
					CustomDomains: []string{"hello.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 8080},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "my-ns", Port: 8080},
					},
				},
			},
			clusterCatalogs:   "testdata/updated-catalog-service-added/catalogs.yaml",
			clusterIngresses:  "testdata/updated-catalog-service-added/ingresses.yaml",
			clusterSecrets:    "testdata/updated-catalog-service-added/secrets.yaml",
			wantCatalogs:      "testdata/updated-catalog-service-added/want.catalogs.yaml",
			wantEdgeIngresses: "testdata/updated-catalog-service-added/want.edge-ingresses.yaml",
			wantIngresses:     "testdata/updated-catalog-service-added/want.ingresses.yaml",
			wantSecrets:       "testdata/updated-catalog-service-added/want.secrets.yaml",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			wantCatalogs := loadFixtures[hubv1alpha1.Catalog](t, test.wantCatalogs)
			wantIngresses := loadFixtures[netv1.Ingress](t, test.wantIngresses)
			wantEdgeIngresses := loadFixtures[hubv1alpha1.EdgeIngress](t, test.wantEdgeIngresses)
			wantSecrets := loadFixtures[corev1.Secret](t, test.wantSecrets)

			clusterCatalogs := loadFixtures[hubv1alpha1.Catalog](t, test.clusterCatalogs)
			clusterIngresses := loadFixtures[netv1.Ingress](t, test.clusterIngresses)
			clusterSecrets := loadFixtures[corev1.Secret](t, test.clusterSecrets)

			var kubeObjects []runtime.Object
			kubeObjects = append(kubeObjects, services...)
			for _, clusterIngress := range clusterIngresses {
				kubeObjects = append(kubeObjects, clusterIngress.DeepCopy())
			}
			for _, secret := range clusterSecrets {
				kubeObjects = append(kubeObjects, secret.DeepCopy())
			}

			var hubObjects []runtime.Object
			for _, clusterCatalog := range clusterCatalogs {
				hubObjects = append(hubObjects, clusterCatalog.DeepCopy())
			}

			kubeClientSet := kubemock.NewSimpleClientset(kubeObjects...)
			hubClientSet := hubkubemock.NewSimpleClientset(hubObjects...)

			ctx, cancel := context.WithCancel(context.Background())

			kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 0)
			hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

			hubInformer.Hub().V1alpha1().Catalogs().Informer()
			kubeInformer.Networking().V1().Ingresses().Informer()

			hubInformer.Start(ctx.Done())
			hubInformer.WaitForCacheSync(ctx.Done())

			kubeInformer.Start(ctx.Done())
			kubeInformer.WaitForCacheSync(ctx.Done())

			client := newPlatformClientMock(t)

			getCatalogCount := 0
			// Cancel the context on the second catalog synchronization occurred.
			client.OnGetCatalogs().TypedReturns(test.platformCatalogs, nil).Run(func(_ mock.Arguments) {
				getCatalogCount++
				if getCatalogCount == 2 {
					cancel()
				}
			})
			client.OnGetWildcardCertificate().
				TypedReturns(edgeingress.Certificate{
					Certificate: []byte("cert"),
					PrivateKey:  []byte("private"),
				}, nil)
			client.OnGetCertificateByDomains(test.platformCatalogs[0].CustomDomains).
				TypedReturns(edgeingress.Certificate{
					Certificate: []byte("cert"),
					PrivateKey:  []byte("private"),
				}, nil)

			oasCh := make(chan struct{})
			oasRegistry := newOasRegistryMock(t)
			oasRegistry.OnUpdated().TypedReturns(oasCh)

			// We are not interested in the output of this function.
			oasRegistry.
				OnGetURL("whoami-2", "default").
				TypedReturns("http://whoami-2.default.svc:8080/spec.json").
				Maybe()
			oasRegistry.
				OnGetURL(mock.Anything, mock.Anything).
				TypedReturns("").
				Maybe()

			w := NewWatcher(client, oasRegistry, kubeClientSet, kubeInformer, hubClientSet, hubInformer, &WatcherConfig{
				IngressClassName:         "ingress-class",
				AgentNamespace:           "agent-ns",
				TraefikCatalogEntryPoint: "catalog-entrypoint",
				TraefikTunnelEntryPoint:  "tunnel-entrypoint",
				DevPortalServiceName:     "dev-portal-service-name",
				DevPortalPort:            8080,
				CatalogSyncInterval:      time.Millisecond,
				CertSyncInterval:         time.Millisecond,
				CertRetryInterval:        time.Millisecond,
			})

			stop := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(stop)
			}()

			<-stop

			catalogList, err := hubClientSet.HubV1alpha1().Catalogs().List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			var catalogs []hubv1alpha1.Catalog
			for _, catalog := range catalogList.Items {
				catalog.Status.SyncedAt = metav1.Time{}

				catalogs = append(catalogs, catalog)
			}

			var ingresses []netv1.Ingress
			for _, namespace := range namespaces {
				var namespaceIngressList *netv1.IngressList
				namespaceIngressList, err = kubeClientSet.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)

				for _, ingress := range namespaceIngressList.Items {
					ingress.Status = netv1.IngressStatus{}

					ingresses = append(ingresses, ingress)
				}
			}

			edgeIngresses, err := hubClientSet.HubV1alpha1().EdgeIngresses("agent-ns").List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			var secrets []corev1.Secret
			for _, namespace := range namespaces {
				var namespaceSecretList *corev1.SecretList
				namespaceSecretList, err = kubeClientSet.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)

				secrets = append(secrets, namespaceSecretList.Items...)
			}

			assert.ElementsMatch(t, wantCatalogs, catalogs)
			assert.ElementsMatch(t, wantIngresses, ingresses)
			assert.ElementsMatch(t, wantEdgeIngresses, edgeIngresses.Items)
			assert.ElementsMatch(t, wantSecrets, secrets)
		})
	}
}

func TestWatcher_Run_OASRegistryUpdated(t *testing.T) {
	kubeClientSet := kubemock.NewSimpleClientset()
	kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 0)

	hubClientSet := hubkubemock.NewSimpleClientset()
	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

	hubInformer.Hub().V1alpha1().Catalogs().Informer()

	ctx, cancel := context.WithCancel(context.Background())

	hubInformer.Start(ctx.Done())
	hubInformer.WaitForCacheSync(ctx.Done())

	kubeInformer.Start(ctx.Done())
	kubeInformer.WaitForCacheSync(ctx.Done())

	client := newPlatformClientMock(t)

	client.OnGetWildcardCertificate().TypedReturns(edgeingress.Certificate{}, nil).Once()

	// Do nothing on the first sync catalogs.
	client.OnGetCatalogs().TypedReturns([]Catalog{}, nil).Once()

	// Make sure catalogs get synced based on a OASRegistry update event.
	// Cancel the context as soon as the first catalog synchronization occurred. This will have
	// the expected effect of finishing the synchronization and stop.
	client.OnGetCatalogs().
		TypedReturns([]Catalog{}, nil).
		Run(func(_ mock.Arguments) { cancel() })

	// Simulate an OpenAPI Spec URL change.
	oasCh := make(chan struct{}, 1)
	oasCh <- struct{}{}

	oasRegistry := newOasRegistryMock(t)
	oasRegistry.OnUpdated().TypedReturns(oasCh)

	w := NewWatcher(client, oasRegistry, kubeClientSet, kubeInformer, hubClientSet, hubInformer, &WatcherConfig{
		IngressClassName:         "ingress-class",
		TraefikCatalogEntryPoint: "entrypoint",
		// Very high interval to prevent the ticker from firing.
		CatalogSyncInterval: time.Hour,
		CertSyncInterval:    time.Hour,
		CertRetryInterval:   time.Hour,
	})

	w.Run(ctx)
}

func loadFixtures[T any](t *testing.T, path string) []T {
	t.Helper()

	if path == "" {
		return []T{}
	}

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(b), 1000)

	var objects []T

	for {
		var object T
		if decoder.Decode(&object) != nil {
			break
		}

		objects = append(objects, object)
	}

	return objects
}