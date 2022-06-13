package acp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

var toUpdate = &hubv1alpha1.AccessControlPolicy{
	ObjectMeta: metav1.ObjectMeta{
		Name: "toUpdate",
	},
	Spec: hubv1alpha1.AccessControlPolicySpec{
		JWT: &hubv1alpha1.AccessControlPolicyJWT{
			PublicKey: "valueToUpdate",
		},
	},
}

var toDelete = &hubv1alpha1.AccessControlPolicy{
	ObjectMeta: metav1.ObjectMeta{
		Name: "toDelete",
	},
	Spec: hubv1alpha1.AccessControlPolicySpec{
		JWT: &hubv1alpha1.AccessControlPolicyJWT{
			PublicKey: "value",
		},
	},
}

func Test_WatcherRun(t *testing.T) {
	clientSetHub := hubkubemock.NewSimpleClientset([]runtime.Object{toUpdate, toDelete}...)

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformer.NewSharedInformerFactory(clientSetHub, 0)
	acpInformer := hubInformer.Hub().V1alpha1().AccessControlPolicies().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), acpInformer.HasSynced)

	var callCount int

	client := newClientMock(t)
	client.OnGetACPs().
		TypedReturns([]ACP{
			{
				Name: "toCreate",
				Config: Config{
					JWT: &jwt.Config{
						PublicKey: "secret",
					},
				},
			},
			{
				Name: "toUpdate",
				Config: Config{
					JWT: &jwt.Config{
						PublicKey: "secretUpdated",
					},
				},
			},
		}, nil).
		Run(func(_ mock.Arguments) {
			callCount++
			if callCount > 1 {
				cancel()
			}
		})

	w := NewWatcher(time.Millisecond, client, clientSetHub, hubInformer)
	go w.Run(ctx)

	<-ctx.Done()

	policy, err := clientSetHub.HubV1alpha1().AccessControlPolicies().Get(ctx, "toCreate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "secret", policy.Spec.JWT.PublicKey)

	policy, err = clientSetHub.HubV1alpha1().AccessControlPolicies().Get(ctx, "toUpdate", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "secretUpdated", policy.Spec.JWT.PublicKey)

	_, err = clientSetHub.HubV1alpha1().AccessControlPolicies().Get(ctx, "toDelete", metav1.GetOptions{})
	require.Error(t, err)
}
