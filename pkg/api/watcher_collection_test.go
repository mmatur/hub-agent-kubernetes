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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

var collectionToUpdate = &hubv1alpha1.APICollection{
	ObjectMeta: metav1.ObjectMeta{
		Name: "collectionToUpdate",
	},
	Spec: hubv1alpha1.APICollectionSpec{
		PathPrefix: "oldPrefix",
	},
}

var collectionToDelete = &hubv1alpha1.APICollection{
	ObjectMeta: metav1.ObjectMeta{
		Name: "collectionToDelete",
	},
	Spec: hubv1alpha1.APICollectionSpec{
		PathPrefix: "oldPrefix",
	},
}

func Test_WatcherCollectionRun(t *testing.T) {
	kubeClientSet := kubefake.NewSimpleClientset()
	clientSetHub := hubfake.NewSimpleClientset([]runtime.Object{collectionToUpdate, collectionToDelete}...)

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformers.NewSharedInformerFactory(clientSetHub, 0)
	collectionInformer := hubInformer.Hub().V1alpha1().APICollections().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), collectionInformer.HasSynced)

	var callCount int

	client := newPlatformClientMock(t)
	client.OnGetCollections().
		TypedReturns([]Collection{
			{
				Name:       "collectionToCreate",
				PathPrefix: "prefix",
				APISelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
				Version: "1",
			},
			{
				Name:       "collectionToUpdate",
				PathPrefix: "prefixUpdate",
				APISelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "newValue"},
				},
				Version: "2",
			},
		}, nil).
		Run(func(_ mock.Arguments) {
			callCount++
			if callCount > 1 {
				cancel()
			}
		})

	w := NewWatcherCollection(client, kubeClientSet, clientSetHub, hubInformer, time.Millisecond)
	go w.Run(ctx)

	<-ctx.Done()

	collection, err := clientSetHub.HubV1alpha1().APICollections().Get(ctx, "collectionToCreate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "prefix", collection.Spec.PathPrefix)
	assert.Equal(t, metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "value"},
	}, collection.Spec.APISelector)

	collection, err = clientSetHub.HubV1alpha1().APICollections().Get(ctx, "collectionToUpdate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "prefixUpdate", collection.Spec.PathPrefix)
	assert.Equal(t, metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "newValue"},
	}, collection.Spec.APISelector)

	_, err = clientSetHub.HubV1alpha1().APICollections().Get(ctx, "collectionToDelete", metav1.GetOptions{})
	require.Error(t, err)
}
