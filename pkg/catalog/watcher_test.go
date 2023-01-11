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

var toUpdate = hubv1alpha1.Catalog{
	ObjectMeta: metav1.ObjectMeta{
		Name: "toUpdate",
		UID:  "uid",
		Annotations: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
	},
	Spec: hubv1alpha1.CatalogSpec{
		Services: []hubv1alpha1.CatalogService{
			{
				Name:       "users",
				Namespace:  "default",
				Port:       8080,
				PathPrefix: "/users",
			},
		},
	},
	Status: hubv1alpha1.CatalogStatus{
		Version:  "version-1",
		SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
		URLs:     "https://sad-bat-456.hub-traefik.io",
		SpecHash: "yxjMx+3w4R4B4YPzoGkqi/g9rLs=",
	},
}

var toDelete = hubv1alpha1.Catalog{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "toDelete",
		Namespace: "default",
		Annotations: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
	},
	Spec: hubv1alpha1.CatalogSpec{
		Services: []hubv1alpha1.CatalogService{
			{
				Name:       "logs",
				Namespace:  "default",
				Port:       8080,
				PathPrefix: "/logs",
			},
		},
	},
	Status: hubv1alpha1.CatalogStatus{
		Version:  "version-1",
		SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
		URLs:     "https://broken-cat-123.hub-traefik.io",
		SpecHash: "7kfh53DLsXumNEaO/nkeVYs/5CI=",
	},
}

func Test_WatcherRun(t *testing.T) {
	clientSetHub := hubkubemock.NewSimpleClientset([]runtime.Object{&toUpdate, &toDelete}...)

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformer.NewSharedInformerFactory(clientSetHub, 0)

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
		{
			Name:    "toUpdate",
			Version: "version-2",
			Domain:  "majestic-beaver-123.hub-traefik.io",
			Services: []hubv1alpha1.CatalogService{
				{
					Name:       "users",
					Namespace:  "default",
					Port:       8080,
					PathPrefix: "/users",
				},
				{
					Name:       "views",
					Namespace:  "default",
					Port:       9000,
					PathPrefix: "/views",
				},
			},
		},
	}

	wantClusterCatalogs := []hubv1alpha1.Catalog{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "toCreate"},
			Spec: hubv1alpha1.CatalogSpec{
				CustomDomains: []string{"hello.example.com"},
				Services:      catalogs[0].Services,
			},
			Status: hubv1alpha1.CatalogStatus{
				Version:  "version-1",
				URLs:     "https://hello.example.com",
				Domains:  []string{"hello.example.com"},
				SpecHash: "sgWOWyiDAbIOGyXnZHF3JnYuWBk=",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "toUpdate"},
			Spec: hubv1alpha1.CatalogSpec{
				Services: catalogs[1].Services,
			},
			Status: hubv1alpha1.CatalogStatus{
				Version:  "version-2",
				URLs:     "https://majestic-beaver-123.hub-traefik.io",
				Domains:  []string{"majestic-beaver-123.hub-traefik.io"},
				SpecHash: "2+eev05bFhgq5v/Vajcl1KJvBaE=",
			},
		},
	}

	client := newPlatformClientMock(t)

	// Cancel the context as soon as the first catalog synchronization occurred.
	client.OnGetCatalogs().TypedReturns(catalogs, nil).Run(func(_ mock.Arguments) { cancel() })

	w := NewWatcher(client, clientSetHub, hubInformer, WatcherConfig{
		CatalogSyncInterval: time.Millisecond,
	})

	w.Run(ctx)

	// Make sure the "toDelete" catalog has been deleted.
	_, err := clientSetHub.HubV1alpha1().Catalogs().Get(ctx, "toDelete", metav1.GetOptions{})
	require.Error(t, err)

	// Make sure the remaining catalogs have the correct shape.
	for i, catalog := range catalogs {
		wantClusterCatalog := wantClusterCatalogs[i]
		assert.Equal(t, wantClusterCatalog.Name, catalog.Name)

		clusterCatalog, errL := clientSetHub.HubV1alpha1().Catalogs().Get(ctx, catalog.Name, metav1.GetOptions{})
		require.NoError(t, errL)

		assert.Equal(t, wantClusterCatalog.Spec, clusterCatalog.Spec)
		assert.WithinDuration(t, time.Now(), clusterCatalog.Status.SyncedAt.Time, time.Second)

		wantClusterCatalog.Status.SyncedAt = clusterCatalog.Status.SyncedAt
		assert.Equal(t, wantClusterCatalog.Status, clusterCatalog.Status)
	}
}
