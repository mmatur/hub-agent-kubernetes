package ingclass

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	neov1alpha1 "github.com/traefik/neo-agent/pkg/crd/api/neo/v1alpha1"
	neoclientset "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/clientset/versioned"
	neokubemock "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/clientset/versioned/fake"
	neoinformer "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/informers/externalversions"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func setupEnv(clientSet clientset.Interface, neoClientSet neoclientset.Interface, watcher *Watcher) error {
	kubeInformer := informers.NewSharedInformerFactoryWithOptions(clientSet, 5*time.Minute)
	kubeInformer.Networking().V1().IngressClasses().Informer().AddEventHandler(watcher)
	kubeInformer.Networking().V1beta1().IngressClasses().Informer().AddEventHandler(watcher)

	ctx := context.Background()
	syncCtx, c := context.WithTimeout(ctx, 5*time.Second)
	defer c()
	kubeInformer.Start(ctx.Done())
	for _, ok := range kubeInformer.WaitForCacheSync(syncCtx.Done()) {
		if !ok {
			return errors.New("informer failed to start")
		}
	}

	neoInformer := neoinformer.NewSharedInformerFactory(neoClientSet, 5*time.Minute)
	neoInformer.Neo().V1alpha1().IngressClasses().Informer().AddEventHandler(watcher)
	neoInformer.Start(ctx.Done())

	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, ok := range neoInformer.WaitForCacheSync(syncCtx.Done()) {
		if !ok {
			return errors.New("informer failed to start")
		}
	}

	return nil
}

func TestWatcher_GetController(t *testing.T) {
	ing := netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{UID: "1", Name: "ing-class-1"},
		Spec: netv1.IngressClassSpec{
			Controller: ControllerTypeTraefik,
		},
	}
	legacyIng := netv1beta1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{UID: "2", Name: "ing-class-2"},
		Spec: netv1beta1.IngressClassSpec{
			Controller: ControllerTypeNginxOfficial,
		},
	}
	customIng := neov1alpha1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{UID: "3", Name: "ing-class-3"},
		Spec: neov1alpha1.IngressClassSpec{
			Controller: ControllerTypeTraefik,
		},
	}
	clientSet := kubemock.NewSimpleClientset(&ing, &legacyIng)
	neoClientSet := neokubemock.NewSimpleClientset(&customIng)
	watcher := NewWatcher()

	err := setupEnv(clientSet, neoClientSet, watcher)
	require.NoError(t, err)

	err = waitForIngressClasses(watcher, 3)
	require.NoError(t, err)

	assert.Equal(t, ControllerTypeTraefik, watcher.GetController("ing-class-1"))
	assert.Equal(t, ControllerTypeNginxOfficial, watcher.GetController("ing-class-2"))
	assert.Equal(t, ControllerTypeTraefik, watcher.GetController("ing-class-3"))

	err = clientSet.NetworkingV1().IngressClasses().Delete(context.Background(), "ing-class-1", metav1.DeleteOptions{})
	require.NoError(t, err)
	err = clientSet.NetworkingV1beta1().IngressClasses().Delete(context.Background(), "ing-class-2", metav1.DeleteOptions{})
	require.NoError(t, err)
	err = neoClientSet.NeoV1alpha1().IngressClasses().Delete(context.Background(), "ing-class-3", metav1.DeleteOptions{})
	require.NoError(t, err)

	err = waitForIngressClasses(watcher, 0)
	require.NoError(t, err)

	assert.Equal(t, "", watcher.GetController("ing-class-1"))
	assert.Equal(t, "", watcher.GetController("ing-class-2"))
	assert.Equal(t, "", watcher.GetController("ing-class-3"))
}

func TestWatcher_GetDefaultController(t *testing.T) {
	tests := []struct {
		desc           string
		ingClass       bool
		legacyIngClass bool
		customIngClass bool
		wantErr        bool
	}{
		{
			desc:     "handles a single IngressClass flagged as default",
			ingClass: true,
			wantErr:  false,
		},
		{
			desc:           "handles a single legacy IngressClass flagged as default",
			legacyIngClass: true,
			wantErr:        false,
		},
		{
			desc:           "handles a single custom IngressClass flagged as default",
			customIngClass: true,
			wantErr:        false,
		},
		{
			desc:           "fails if there more than one IngressClass is flagged as default",
			ingClass:       true,
			legacyIngClass: true,
			wantErr:        true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var (
				resources       []runtime.Object
				customResources []runtime.Object
			)
			if test.ingClass {
				resources = append(resources, &netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						UID:         "1",
						Name:        "ing-class-1",
						Annotations: map[string]string{annotationDefaultIngressClass: "true"},
					},
					Spec: netv1.IngressClassSpec{Controller: ControllerTypeTraefik},
				})
			}
			if test.legacyIngClass {
				resources = append(resources, &netv1beta1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						UID:         "2",
						Name:        "ing-class-2",
						Annotations: map[string]string{annotationDefaultIngressClass: "true"},
					},
					Spec: netv1beta1.IngressClassSpec{Controller: ControllerTypeTraefik},
				})
			}
			if test.customIngClass {
				customResources = append(customResources, &neov1alpha1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						UID:         "3",
						Name:        "ing-class-3",
						Annotations: map[string]string{annotationDefaultIngressClass: "true"},
					},
					Spec: neov1alpha1.IngressClassSpec{Controller: ControllerTypeTraefik},
				})
			}

			clientSet := kubemock.NewSimpleClientset(resources...)
			neoClientSet := neokubemock.NewSimpleClientset(customResources...)
			watcher := NewWatcher()
			err := setupEnv(clientSet, neoClientSet, watcher)
			require.NoError(t, err)

			err = waitForIngressClasses(watcher, len(resources)+len(customResources))
			require.NoError(t, err)

			defaultCtrl, err := watcher.GetDefaultController()
			if test.wantErr {
				assert.Error(t, err)
				assert.Empty(t, defaultCtrl)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, ControllerTypeTraefik, defaultCtrl)
		})
	}
}

func waitForIngressClasses(watcher *Watcher, length int) error {
	done := make(chan struct{})
	go func() {
		for {
			watcher.mu.RLock()
			l := len(watcher.ingressClasses)
			watcher.mu.RUnlock()
			if l == length {
				break
			}

			// Show some mercy for CPUs.
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		return errors.New("timed out waiting for ingress classes")
	}
	return nil
}
