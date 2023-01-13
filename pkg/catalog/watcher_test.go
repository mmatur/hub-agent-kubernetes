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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

func Test_WatcherRun(t *testing.T) {
	tests := []struct {
		desc             string
		platformCatalogs []Catalog
		clusterCatalogs  []runtime.Object

		wantCatalogs []hubv1alpha1.Catalog
	}{
		{
			desc: "new catalog present on the platform",
			platformCatalogs: []Catalog{
				{
					Name:          "new-catalog",
					Version:       "version-1",
					CustomDomains: []string{"hello.example.com", "welcome.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
						{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
					},
				},
			},
			wantCatalogs: []hubv1alpha1.Catalog{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "new-catalog"},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"hello.example.com", "welcome.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
							{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
							{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
						},
					},
					Status: hubv1alpha1.CatalogStatus{
						Version:  "version-1",
						Domains:  []string{"hello.example.com", "welcome.example.com"},
						URLs:     "https://hello.example.com,https://welcome.example.com",
						SpecHash: "HbhRY3LGNcaqKPJ+wmFo7lUwj5I=",
						Services: []hubv1alpha1.CatalogServiceStatus{
							{Name: "whoami-1", Namespace: "default"},
							{Name: "whoami-2", Namespace: "default", OpenAPISpecURL: "http://whoami-2.default.svc:8080/spec.json"},
							{Name: "whoami-3", Namespace: "my-ns"},
						},
					},
				},
			},
		},
		{
			desc: "catalog updated",
			platformCatalogs: []Catalog{
				{
					Name:          "catalog",
					Version:       "version-2",
					CustomDomains: []string{"hello.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 8080, OpenAPISpecURL: "http://hello.example.com/spec.json"},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
					},
				},
			},
			clusterCatalogs: []runtime.Object{
				&hubv1alpha1.Catalog{
					ObjectMeta: metav1.ObjectMeta{Name: "catalog"},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"hello.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
							{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
							{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
						},
					},
					Status: hubv1alpha1.CatalogStatus{
						Version:  "version-1",
						Domains:  []string{"hello.example.com"},
						URLs:     "https://hello.example.com",
						SpecHash: "3oh1v5LUNVT5Xh01nzFNNyCTCTc=",
						Services: []hubv1alpha1.CatalogServiceStatus{
							{Name: "whoami-1", Namespace: "default"},
							{Name: "whoami-2", Namespace: "default", OpenAPISpecURL: "http://whoami-2.default.svc:8080/spec.json"},
							{Name: "whoami-3", Namespace: "my-ns"},
						},
					},
				},
			},
			wantCatalogs: []hubv1alpha1.Catalog{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "catalog"},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"hello.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 8080, OpenAPISpecURL: "http://hello.example.com/spec.json"},
							{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
						},
					},
					Status: hubv1alpha1.CatalogStatus{
						Version:  "version-2",
						Domains:  []string{"hello.example.com"},
						URLs:     "https://hello.example.com",
						SpecHash: "JiNFWTDh2QN2UXI2axjtY21Zpf0=",
						Services: []hubv1alpha1.CatalogServiceStatus{
							{Name: "whoami-1", Namespace: "default", OpenAPISpecURL: "http://hello.example.com/spec.json"},
							{Name: "whoami-2", Namespace: "default", OpenAPISpecURL: "http://whoami-2.default.svc:8080/spec.json"},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			hubClientSet := hubkubemock.NewSimpleClientset(test.clusterCatalogs...)

			hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

			hubInformer.Hub().V1alpha1().Catalogs().Informer()

			ctx, cancel := context.WithCancel(context.Background())

			hubInformer.Start(ctx.Done())
			hubInformer.WaitForCacheSync(ctx.Done())

			client := newPlatformClientMock(t)

			getCatalogCount := 0
			// Cancel the context on the second catalog synchronization occurred.
			client.OnGetCatalogs().TypedReturns(test.platformCatalogs, nil).Run(func(_ mock.Arguments) {
				getCatalogCount++
				if getCatalogCount == 2 {
					cancel()
				}
			})

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

			w := NewWatcher(client, oasRegistry, hubClientSet, hubInformer, WatcherConfig{
				CatalogSyncInterval:  time.Millisecond,
				AgentNamespace:       "agent-ns",
				DevPortalServiceName: "dev-portal-service-name",
				DevPortalPort:        8080,
			})

			w.Run(ctx)

			catalogList, err := hubClientSet.HubV1alpha1().Catalogs().List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			var catalogs []hubv1alpha1.Catalog
			for _, catalog := range catalogList.Items {
				catalog.TypeMeta = metav1.TypeMeta{}
				catalog.Status.SyncedAt = metav1.Time{}

				catalogs = append(catalogs, catalog)
			}

			assert.ElementsMatch(t, test.wantCatalogs, catalogs)
		})
	}
}

func Test_WatcherRun_syncChild(t *testing.T) {
	hubClientSet := hubkubemock.NewSimpleClientset()

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

	catalogInformer := hubInformer.Hub().V1alpha1().Catalogs().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), catalogInformer.HasSynced)

	catalogs := []Catalog{
		{
			Name:          "toCreate",
			Version:       "version-1",
			CustomDomains: []string{"hello.example.com"},
			Domain:        "majestic-beaver-123.hub-traefik.io",
			Services: []hubv1alpha1.CatalogService{
				{
					Name:       "views",
					Namespace:  "default",
					Port:       8080,
					PathPrefix: "/views",
				},
			},
		},
	}

	wantClusterCatalog := hubv1alpha1.Catalog{
		ObjectMeta: metav1.ObjectMeta{Name: "toCreate"},
		Spec: hubv1alpha1.CatalogSpec{
			CustomDomains: []string{"hello.example.com"},
			Services:      catalogs[0].Services,
		},
		Status: hubv1alpha1.CatalogStatus{
			Version:  "version-1",
			URLs:     "https://hello.example.com",
			Domains:  []string{"hello.example.com"},
			SpecHash: "+MBiHj7QfVFo+CKOe2AdHrOqatM=",
			Services: []hubv1alpha1.CatalogServiceStatus{
				{
					Name:      "views",
					Namespace: "default",
				},
			},
		},
	}

	client := newPlatformClientMock(t)

	getCatalogCount := 0
	// Cancel the context on the second catalog synchronization occurred.
	client.OnGetCatalogs().TypedReturns(catalogs, nil).Run(func(_ mock.Arguments) {
		getCatalogCount++
		if getCatalogCount == 2 {
			cancel()
		}
	})

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

	w := NewWatcher(client, oasRegistry, hubClientSet, hubInformer, WatcherConfig{
		CatalogSyncInterval:  time.Millisecond,
		AgentNamespace:       "agent-ns",
		DevPortalServiceName: "dev-portal-service-name",
		DevPortalPort:        8080,
	})

	w.Run(ctx)

	// Make sure the remaining catalogs have the correct shape.
	clusterCatalog, errL := hubClientSet.HubV1alpha1().Catalogs().Get(ctx, "toCreate", metav1.GetOptions{})
	require.NoError(t, errL)

	assert.Equal(t, wantClusterCatalog.Spec, clusterCatalog.Spec)
	assert.WithinDuration(t, time.Now(), clusterCatalog.Status.SyncedAt.Time, time.Second)

	wantClusterCatalog.Status.SyncedAt = clusterCatalog.Status.SyncedAt
	assert.Equal(t, wantClusterCatalog.Status, clusterCatalog.Status)

	name, err := getEdgeIngressName("toCreate")
	require.NoError(t, err)

	wantClusterEdgeIngress := hubv1alpha1.EdgeIngress{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "agent-ns",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "hub.traefik.io/v1alpha1",
					Kind:       "Catalog",
					Name:       clusterCatalog.Name,
					UID:        clusterCatalog.UID,
				},
			},
		},
		Spec: hubv1alpha1.EdgeIngressSpec{
			Service: hubv1alpha1.EdgeIngressService{
				Name: "dev-portal-service-name",
				Port: 8080,
			},
		},
		Status: hubv1alpha1.EdgeIngressStatus{},
	}

	edgeIngress, err := hubClientSet.HubV1alpha1().EdgeIngresses("agent-ns").Get(ctx, name, metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, wantClusterEdgeIngress.Spec, edgeIngress.Spec)
}

func TestWatcher_Run_OASRegistryUpdated(t *testing.T) {
	hubClientSet := hubkubemock.NewSimpleClientset()

	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

	hubInformer.Hub().V1alpha1().Catalogs().Informer()

	ctx, cancel := context.WithCancel(context.Background())

	hubInformer.Start(ctx.Done())
	hubInformer.WaitForCacheSync(ctx.Done())

	client := newPlatformClientMock(t)

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

	w := NewWatcher(client, oasRegistry, hubClientSet, hubInformer, WatcherConfig{
		// Very high interval to prevent the ticker from firing.
		CatalogSyncInterval: time.Hour,
	})

	w.Run(ctx)
}
