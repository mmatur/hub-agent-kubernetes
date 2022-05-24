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
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
)

var toUpdate = hubv1alpha1.EdgeIngress{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "toUpdate",
		Namespace: "default",
		UID:       "uid",
		Annotations: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
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
		Annotations: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
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
	clientSet := kubemock.NewSimpleClientset()

	ctx, cancel := context.WithCancel(context.Background())
	hubInformer := hubinformer.NewSharedInformerFactory(clientSetHub, 0)

	edgeIngressInformer := hubInformer.Hub().V1alpha1().EdgeIngresses().Informer()

	hubInformer.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), edgeIngressInformer.HasSynced)

	edgeIngresses := []EdgeIngress{
		{
			Name:      "toCreate",
			Namespace: "default",
			Domain:    "majestic-beaver-123.hub-traefik.io",
			Version:   "version-1",
			Service:   Service{Name: "service-1", Port: 8080},
			ACP:       &ACP{Name: "acp-name"},
		},
		{
			Name:      "toUpdate",
			Namespace: "default",
			Domain:    "sad-bat-123.hub-traefik.io",
			Version:   "version-2",
			Service:   Service{Name: "service-2", Port: 8082},
			ACP:       &ACP{Name: "acp-name"},
		},
	}

	hashes := map[string]string{
		"toCreate": "gFO9z9bw0btZf3+lNIiaXfA8z0g=",
		"toUpdate": "4vJBrpeDJLuGzikpIg0ZJTca9FQ=",
	}

	var callCount int
	client := clientMock{
		getCertificateFunc: func() (Certificate, error) {
			return Certificate{
				Certificate: []byte("cert"),
				PrivateKey:  []byte("private"),
			}, nil
		},
		getEdgeIngressesFunc: func() ([]EdgeIngress, error) {
			callCount++

			if callCount > 1 {
				cancel()
			}

			return edgeIngresses, nil
		},
	}

	traefikClientSet := traefikkubemock.NewSimpleClientset()

	w, err := NewWatcher(client, clientSetHub, clientSet, traefikClientSet.TraefikV1alpha1(), hubInformer, WatcherConfig{
		IngressClassName:        "traefik-hub",
		TraefikEntryPoint:       "traefikhub-tunl",
		AgentNamespace:          "hub-agent",
		EdgeIngressSyncInterval: time.Millisecond,
		CertRetryInterval:       time.Millisecond,
		CertSyncInterval:        time.Millisecond,
	})

	require.NoError(t, err)

	stop := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(stop)
	}()

	<-stop

	_, err = clientSetHub.HubV1alpha1().
		EdgeIngresses("default").
		Get(ctx, "toDelete", metav1.GetOptions{})
	require.Error(t, err)

	for _, edgeIngress := range edgeIngresses {
		edgeIng, errL := clientSetHub.HubV1alpha1().
			EdgeIngresses(edgeIngress.Namespace).
			Get(ctx, edgeIngress.Name, metav1.GetOptions{})
		require.NoError(t, errL)

		assert.Equal(t, hubv1alpha1.EdgeIngressSpec{
			Service: hubv1alpha1.EdgeIngressService{
				Name: edgeIngress.Service.Name,
				Port: edgeIngress.Service.Port,
			},
			ACP: &hubv1alpha1.EdgeIngressACP{
				Name: edgeIngress.ACP.Name,
			},
		}, edgeIng.Spec)

		assert.WithinDuration(t, time.Now(), edgeIng.Status.SyncedAt.Time, 100*time.Millisecond)
		edgeIng.Status.SyncedAt = metav1.Time{}

		assert.Equal(t, hubv1alpha1.EdgeIngressStatus{
			Version:    edgeIngress.Version,
			SyncedAt:   metav1.Time{},
			Domain:     edgeIngress.Domain,
			URL:        "https://" + edgeIngress.Domain,
			SpecHash:   hashes[edgeIngress.Name],
			Connection: hubv1alpha1.EdgeIngressConnectionUp,
		}, edgeIng.Status)

		// Make sure the ingress related to the edgeIngress is created.
		ctx = context.Background()
		ing, errL := clientSet.NetworkingV1().Ingresses(edgeIngress.Namespace).Get(ctx, edgeIngress.Name, metav1.GetOptions{})
		require.NoError(t, errL)

		assert.Equal(t, map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		}, ing.ObjectMeta.Labels)

		assert.Equal(t, map[string]string{
			"hub.traefik.io/access-control-policy":             "acp-name",
			"traefik.ingress.kubernetes.io/router.tls":         "true",
			"traefik.ingress.kubernetes.io/router.entrypoints": "traefikhub-tunl",
		}, ing.ObjectMeta.Annotations)

		assert.Equal(t, []metav1.OwnerReference{
			{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "EdgeIngress",
				Name:       edgeIng.Name,
				UID:        edgeIng.UID,
			},
		}, ing.ObjectMeta.OwnerReferences)

		wantPathType := netv1.PathTypePrefix
		assert.Equal(t, netv1.IngressSpec{
			IngressClassName: pointer.StringPtr("traefik-hub"),
			TLS: []netv1.IngressTLS{
				{
					Hosts: []string{edgeIngress.Domain},
				},
			},
			Rules: []netv1.IngressRule{
				{
					Host: edgeIngress.Domain,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &wantPathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: edgeIngress.Service.Name,
											Port: netv1.ServiceBackendPort{
												Number: int32(edgeIngress.Service.Port),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, ing.Spec)
	}
}

type clientMock struct {
	getEdgeIngressesFunc func() ([]EdgeIngress, error)
	getCertificateFunc   func() (Certificate, error)
}

func (c clientMock) GetEdgeIngresses(_ context.Context) ([]EdgeIngress, error) {
	return c.getEdgeIngressesFunc()
}

func (c clientMock) GetCertificate(_ context.Context) (Certificate, error) {
	return c.getCertificateFunc()
}
