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

var accessToUpdate = &hubv1alpha1.APIAccess{
	ObjectMeta: metav1.ObjectMeta{
		Name: "accessToUpdate",
	},
	Spec: hubv1alpha1.APIAccessSpec{
		Groups: []string{"group"},
		APISelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"key": "value"},
		},
		APICollectionSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"key": "value"},
		},
	},
}

var accessToDelete = &hubv1alpha1.APIAccess{
	ObjectMeta: metav1.ObjectMeta{
		Name: "accessToDelete",
	},
	Spec: hubv1alpha1.APIAccessSpec{
		Groups: []string{"group"},
		APISelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"key": "value"},
		},
	},
}

func Test_WatcherAccessRun(t *testing.T) {
	kubeClientSet := kubefake.NewSimpleClientset()
	clientSetHub := hubfake.NewSimpleClientset([]runtime.Object{accessToUpdate, accessToDelete}...)

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformers.NewSharedInformerFactory(clientSetHub, 0)
	accessInformer := hubInformer.Hub().V1alpha1().APIAccesses().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), accessInformer.HasSynced)

	var callCount int

	client := newPlatformClientMock(t)
	client.OnGetAccesses().
		TypedReturns([]Access{
			{
				Name:   "accessToCreate",
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"create-key-api": "value"},
				},
				APICollectionSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"create-key-collection": "value"},
				},
				Version: "1",
			},
			{
				Name:   "accessToUpdate",
				Groups: []string{"group-updated"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"update-key-api": "value"},
				},
				APICollectionSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"update-key-collection": "value"},
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

	w := NewWatcherAccess(client, kubeClientSet, clientSetHub, hubInformer, time.Millisecond)
	go w.Run(ctx)

	<-ctx.Done()

	api, err := clientSetHub.HubV1alpha1().APIAccesses().Get(ctx, "accessToCreate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, hubv1alpha1.APIAccessSpec{
		Groups: []string{"group"},
		APISelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"create-key-api": "value"},
		},
		APICollectionSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"create-key-collection": "value"},
		},
	}, api.Spec)

	api, err = clientSetHub.HubV1alpha1().APIAccesses().Get(ctx, "accessToUpdate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, hubv1alpha1.APIAccessSpec{
		Groups: []string{"group-updated"},
		APISelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"update-key-api": "value"},
		},
		APICollectionSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"update-key-collection": "value"},
		},
	}, api.Spec)

	_, err = clientSetHub.HubV1alpha1().APIAccesses().Get(ctx, "accessToDelete", metav1.GetOptions{})
	require.Error(t, err)
}
