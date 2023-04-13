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

var apiToUpdate = &hubv1alpha1.API{
	ObjectMeta: metav1.ObjectMeta{
		Name: "apiToUpdate",
	},
	Spec: hubv1alpha1.APISpec{
		PathPrefix: "oldPrefix",
	},
}

var apiToDelete = &hubv1alpha1.API{
	ObjectMeta: metav1.ObjectMeta{
		Name: "apiToDelete",
	},
	Spec: hubv1alpha1.APISpec{
		PathPrefix: "oldPrefix",
	},
}

func Test_WatcherAPIRun(t *testing.T) {
	kubeClientSet := kubefake.NewSimpleClientset()
	clientSetHub := hubfake.NewSimpleClientset([]runtime.Object{apiToUpdate, apiToDelete}...)

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformers.NewSharedInformerFactory(clientSetHub, 0)
	apiInformer := hubInformer.Hub().V1alpha1().APIs().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), apiInformer.HasSynced)

	var callCount int

	client := newPlatformClientMock(t)
	client.OnGetAPIs().
		TypedReturns([]API{
			{
				Name:       "apiToCreate",
				Labels:     map[string]string{"foo": "bar"},
				PathPrefix: "prefix",
				Service: Service{
					Name: "service",
					Port: 80,
				},
				Version: "1",
			},
			{
				Name:       "apiToUpdate",
				Labels:     map[string]string{"foo": "bar"},
				PathPrefix: "prefixUpdate",
				Service: Service{
					Name: "serviceUpdate",
					Port: 80,
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

	w := NewWatcherAPI(client, kubeClientSet, clientSetHub, hubInformer, time.Millisecond)
	go w.Run(ctx)

	<-ctx.Done()

	api, err := clientSetHub.HubV1alpha1().APIs("").Get(ctx, "apiToCreate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "prefix", api.Spec.PathPrefix)
	assert.Equal(t, hubv1alpha1.APIService{
		Name: "service",
		Port: hubv1alpha1.APIServiceBackendPort{
			Number: 80,
		},
	}, api.Spec.Service)
	assert.Equal(t, map[string]string{"foo": "bar"}, api.Labels)

	api, err = clientSetHub.HubV1alpha1().APIs("").Get(ctx, "apiToUpdate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "prefixUpdate", api.Spec.PathPrefix)
	assert.Equal(t, hubv1alpha1.APIService{
		Name: "serviceUpdate",
		Port: hubv1alpha1.APIServiceBackendPort{
			Number: 80,
		},
	}, api.Spec.Service)
	assert.Equal(t, map[string]string{"foo": "bar"}, api.Labels)

	_, err = clientSetHub.HubV1alpha1().APIs("").Get(ctx, "apiToDelete", metav1.GetOptions{})
	require.Error(t, err)
}
