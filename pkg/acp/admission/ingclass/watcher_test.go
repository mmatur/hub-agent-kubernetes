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

package ingclass

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func setupEnv(clientSet clientset.Interface, hubClientSet hubclientset.Interface, watcher *Watcher) error {
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

	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 5*time.Minute)
	hubInformer.Hub().V1alpha1().IngressClasses().Informer().AddEventHandler(watcher)
	hubInformer.Start(ctx.Done())

	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, ok := range hubInformer.WaitForCacheSync(syncCtx.Done()) {
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
			Controller: ControllerTypeTraefik,
		},
	}
	customIng := hubv1alpha1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{UID: "3", Name: "ing-class-3"},
		Spec: hubv1alpha1.IngressClassSpec{
			Controller: ControllerTypeTraefik,
		},
	}
	clientSet := kubemock.NewSimpleClientset(&ing, &legacyIng)
	hubClientSet := hubkubemock.NewSimpleClientset(&customIng)
	watcher := NewWatcher()

	err := setupEnv(clientSet, hubClientSet, watcher)
	require.NoError(t, err)

	err = waitForIngressClasses(watcher, 3)
	require.NoError(t, err)

	ctrlr, err := watcher.GetController("ing-class-1")
	assert.NoError(t, err)
	assert.Equal(t, ControllerTypeTraefik, ctrlr)
	ctrlr, err = watcher.GetController("ing-class-2")
	assert.NoError(t, err)
	assert.Equal(t, ControllerTypeTraefik, ctrlr)
	ctrlr, err = watcher.GetController("ing-class-3")
	assert.NoError(t, err)
	assert.Equal(t, ControllerTypeTraefik, ctrlr)

	err = clientSet.NetworkingV1().IngressClasses().Delete(context.Background(), "ing-class-1", metav1.DeleteOptions{})
	require.NoError(t, err)
	err = clientSet.NetworkingV1beta1().IngressClasses().Delete(context.Background(), "ing-class-2", metav1.DeleteOptions{})
	require.NoError(t, err)
	err = hubClientSet.HubV1alpha1().IngressClasses().Delete(context.Background(), "ing-class-3", metav1.DeleteOptions{})
	require.NoError(t, err)

	err = waitForIngressClasses(watcher, 0)
	require.NoError(t, err)

	ctrlr, err = watcher.GetController("ing-class-1")
	assert.Error(t, err)
	assert.Equal(t, "", ctrlr)
	ctrlr, err = watcher.GetController("ing-class-2")
	assert.Error(t, err)
	assert.Equal(t, "", ctrlr)
	ctrlr, err = watcher.GetController("ing-class-3")
	assert.Error(t, err)
	assert.Equal(t, "", ctrlr)
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
				customResources = append(customResources, &hubv1alpha1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						UID:         "3",
						Name:        "ing-class-3",
						Annotations: map[string]string{annotationDefaultIngressClass: "true"},
					},
					Spec: hubv1alpha1.IngressClassSpec{Controller: ControllerTypeTraefik},
				})
			}

			clientSet := kubemock.NewSimpleClientset(resources...)
			hubClientSet := hubkubemock.NewSimpleClientset(customResources...)
			watcher := NewWatcher()
			err := setupEnv(clientSet, hubClientSet, watcher)
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
