package edgeingress

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

var toUpdate = hubv1alpha1.EdgeIngress{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "toUpdate",
		Namespace: "default",
	},
	Spec: hubv1alpha1.EdgeIngressSpec{
		Service: hubv1alpha1.EdgeIngressService{
			Name: "service-1",
			Port: 8081,
		},
	},
	Status: hubv1alpha1.EdgeIngressStatus{
		Version:    "version-1",
		SyncedAt:   metav1.NewTime(time.Now().Add(-time.Hour)),
		URL:        "https://sad-bat-456.hub-traefik.io",
		SpecHash:   "yxjMx+3w4R4B4YPzoGkqi/g9rLs=",
		Connection: hubv1alpha1.EdgeIngressConnectionDown,
	},
}

var toDelete = hubv1alpha1.EdgeIngress{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "toDelete",
		Namespace: "default",
	},
	Spec: hubv1alpha1.EdgeIngressSpec{
		Service: hubv1alpha1.EdgeIngressService{
			Name: "service-3",
			Port: 8080,
		},
	},
	Status: hubv1alpha1.EdgeIngressStatus{
		Version:    "version-1",
		SyncedAt:   metav1.NewTime(time.Now().Add(-time.Hour)),
		URL:        "https://broken-cat-123.hub-traefik.io",
		SpecHash:   "7kfh53DLsXumNEaO/nkeVYs/5CI=",
		Connection: hubv1alpha1.EdgeIngressConnectionDown,
	},
}

func Test_WatcherRun(t *testing.T) {
	clientSetHub := hubkubemock.NewSimpleClientset([]runtime.Object{&toUpdate, &toDelete}...)

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformer.NewSharedInformerFactory(clientSetHub, 0)

	edgeIngressInformer := hubInformer.Hub().V1alpha1().EdgeIngresses().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), edgeIngressInformer.HasSynced)

	var callCount int
	client := clientMock{
		getEdgeIngressesFunc: func() ([]EdgeIngress, error) {
			callCount++

			if callCount > 1 {
				cancel()
			}

			return []EdgeIngress{
				{
					Name:         "toCreate",
					Namespace:    "default",
					Domain:       "majestic-beaver-123.hub-traefik.io",
					Version:      "version-1",
					ServiceName:  "service-1",
					ServicePort:  8080,
					ACPName:      "acp",
					ACPNamespace: "default",
				},
				{
					Name:         "toUpdate",
					Namespace:    "default",
					Domain:       "sad-bat-123.hub-traefik.io",
					Version:      "version-2",
					ServiceName:  "service-2",
					ServicePort:  8082,
					ACPName:      "acp",
					ACPNamespace: "default",
				},
			}, nil
		},
	}
	w := NewWatcher(time.Millisecond, client, clientSetHub, hubInformer)
	go w.Run(ctx)

	<-ctx.Done()

	// Make sure the EdgeIngress to create has been created.
	edgeIng, err := clientSetHub.HubV1alpha1().
		EdgeIngresses("default").
		Get(ctx, "toCreate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, hubv1alpha1.EdgeIngressSpec{
		Service: hubv1alpha1.EdgeIngressService{
			Name: "service-1",
			Port: 8080,
		},
		ACP: &hubv1alpha1.EdgeIngressACP{
			Name:      "acp",
			Namespace: "default",
		},
	}, edgeIng.Spec)

	assert.WithinDuration(t, time.Now(), edgeIng.Status.SyncedAt.Time, 100*time.Millisecond)
	edgeIng.Status.SyncedAt = metav1.Time{}

	assert.Equal(t, hubv1alpha1.EdgeIngressStatus{
		Version:    "version-1",
		SyncedAt:   metav1.Time{},
		Domain:     "majestic-beaver-123.hub-traefik.io",
		URL:        "https://majestic-beaver-123.hub-traefik.io",
		SpecHash:   "gmFASysBkrYjEOkykBdT7mAXMl8=",
		Connection: hubv1alpha1.EdgeIngressConnectionDown,
	}, edgeIng.Status)

	// Make sure the EdgeIngress to update has been updated.
	edgeIng, err = clientSetHub.HubV1alpha1().
		EdgeIngresses("default").
		Get(ctx, "toUpdate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, hubv1alpha1.EdgeIngressSpec{
		Service: hubv1alpha1.EdgeIngressService{
			Name: "service-2",
			Port: 8082,
		},
		ACP: &hubv1alpha1.EdgeIngressACP{
			Name:      "acp",
			Namespace: "default",
		},
	}, edgeIng.Spec)

	assert.WithinDuration(t, time.Now(), edgeIng.Status.SyncedAt.Time, 100*time.Millisecond)
	edgeIng.Status.SyncedAt = metav1.Time{}

	assert.Equal(t, hubv1alpha1.EdgeIngressStatus{
		Version:    "version-2",
		SyncedAt:   metav1.Time{},
		Domain:     "sad-bat-123.hub-traefik.io",
		URL:        "https://sad-bat-123.hub-traefik.io",
		SpecHash:   "WXzOAUMsFW9vmfzKxCXI14wlJjE=",
		Connection: hubv1alpha1.EdgeIngressConnectionDown,
	}, edgeIng.Status)

	_, err = clientSetHub.HubV1alpha1().
		EdgeIngresses("default").
		Get(ctx, "toDelete", metav1.GetOptions{})
	require.Error(t, err)
}

type clientMock struct {
	getEdgeIngressesFunc func() ([]EdgeIngress, error)
}

func (c clientMock) GetEdgeIngresses(_ context.Context) ([]EdgeIngress, error) {
	return c.getEdgeIngressesFunc()
}
