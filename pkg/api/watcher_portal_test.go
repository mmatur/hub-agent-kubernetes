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
	"bytes"
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/informers"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func Test_WatcherRun(t *testing.T) {
	services := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "whoami-2",
				Namespace: "default",
				Annotations: map[string]string{
					"hub.traefik.io/openapi-path": "/spec.json",
					"hub.traefik.io/openapi-port": "8080",
				},
			},
		},
	}

	tests := []struct {
		desc            string
		platformPortals []Portal

		clusterPortals       string
		clusterEdgeIngresses string

		wantPortals       string
		wantEdgeIngresses string
	}{
		{
			desc: "new portal present on the platform needs to be created on the cluster",
			platformPortals: []Portal{
				{
					Name:        "new-portal",
					Description: "My new portal",
					Gateway:     "gateway",
					Version:     "version-1",
					CustomDomains: []string{
						"hello.example.com",
						"welcome.example.com",
					},
				},
			},
			wantPortals:       "testdata/new-portal/want.portals.yaml",
			wantEdgeIngresses: "testdata/new-portal/want.edge-ingresses.yaml",
		},
		{
			desc: "modified portal on the platform needs to be updated on the cluster",
			platformPortals: []Portal{
				{
					Name:        "modified-portal",
					Description: "My modified portal",
					Gateway:     "modified-gateway",
					Version:     "version-2",
					CustomDomains: []string{
						"hello.example.com",
						"new.example.com",
					},
				},
			},
			clusterPortals:       "testdata/update-portal/portals.yaml",
			clusterEdgeIngresses: "testdata/update-portal/edge-ingresses.yaml",
			wantPortals:          "testdata/update-portal/want.portals.yaml",
			wantEdgeIngresses:    "testdata/update-portal/want.edge-ingresses.yaml",
		},
		{
			desc:                 "deleted portal on the platform needs to be deleted on the cluster",
			platformPortals:      []Portal{},
			clusterPortals:       "testdata/delete-portal/portals.yaml",
			clusterEdgeIngresses: "testdata/delete-portal/edge-ingresses.yaml",
			wantEdgeIngresses:    "testdata/delete-portal/want.edge-ingresses.yaml",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			wantPortals := loadFixtures[hubv1alpha1.APIPortal](t, test.wantPortals)
			wantEdgeIngresses := loadFixtures[hubv1alpha1.EdgeIngress](t, test.wantEdgeIngresses)

			clusterPortals := loadFixtures[hubv1alpha1.APIPortal](t, test.clusterPortals)
			clusterEdgeIngresses := loadFixtures[hubv1alpha1.EdgeIngress](t, test.clusterEdgeIngresses)

			var kubeObjects []runtime.Object
			kubeObjects = append(kubeObjects, services...)

			var hubObjects []runtime.Object
			for _, clusterPortal := range clusterPortals {
				hubObjects = append(hubObjects, clusterPortal.DeepCopy())
			}
			for _, clusterEdgeIngress := range clusterEdgeIngresses {
				hubObjects = append(hubObjects, clusterEdgeIngress.DeepCopy())
			}

			kubeClientSet := kubemock.NewSimpleClientset(kubeObjects...)
			hubClientSet := hubkubemock.NewSimpleClientset(hubObjects...)

			ctx, cancel := context.WithCancel(context.Background())

			kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 0)
			hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

			hubInformer.Hub().V1alpha1().APIPortals().Informer()
			kubeInformer.Networking().V1().Ingresses().Informer()

			hubInformer.Start(ctx.Done())
			hubInformer.WaitForCacheSync(ctx.Done())

			kubeInformer.Start(ctx.Done())
			kubeInformer.WaitForCacheSync(ctx.Done())

			client := newPlatformClientMock(t)

			getPortalCount := 0
			// Cancel the context on the second portal synchronization occurred.
			client.OnGetPortals().TypedReturns(test.platformPortals, nil).Run(func(_ mock.Arguments) {
				getPortalCount++
				if getPortalCount == 2 {
					cancel()
				}
			})

			w := NewWatcherPortal(client, kubeClientSet, kubeInformer, hubClientSet, hubInformer, &WatcherPortalConfig{
				IngressClassName:        "ingress-class",
				AgentNamespace:          "agent-ns",
				TraefikAPIEntryPoint:    "api-entrypoint",
				TraefikTunnelEntryPoint: "tunnel-entrypoint",
				DevPortalServiceName:    "dev-portal-service-name",
				DevPortalPort:           8080,
				PortalSyncInterval:      time.Millisecond,
			})

			stop := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(stop)
			}()

			<-stop

			assertPortalsMatches(t, hubClientSet, wantPortals)
			assertEdgeIngressesMatches(t, hubClientSet, "agent-ns", wantEdgeIngresses)
		})
	}
}

func assertPortalsMatches(t *testing.T, hubClientSet *hubkubemock.Clientset, want []hubv1alpha1.APIPortal) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	portalList, err := hubClientSet.HubV1alpha1().APIPortals().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var portals []hubv1alpha1.APIPortal
	for _, portal := range portalList.Items {
		portal.Status.SyncedAt = metav1.Time{}

		portals = append(portals, portal)
	}

	sort.Slice(portals, func(i, j int) bool {
		return portals[i].Name < portals[j].Name
	})

	assert.Equal(t, want, portals)
}

func assertEdgeIngressesMatches(t *testing.T, hubClientSet *hubkubemock.Clientset, namespace string, want []hubv1alpha1.EdgeIngress) {
	t.Helper()

	sort.Slice(want, func(i, j int) bool {
		return want[i].Name < want[j].Name
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	edgeIngresses, err := hubClientSet.HubV1alpha1().EdgeIngresses(namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	var gotEdgeIngresses []hubv1alpha1.EdgeIngress
	for _, portal := range edgeIngresses.Items {
		portal.Status.SyncedAt = metav1.Time{}

		gotEdgeIngresses = append(gotEdgeIngresses, portal)
	}

	sort.Slice(gotEdgeIngresses, func(i, j int) bool {
		return gotEdgeIngresses[i].Name < gotEdgeIngresses[j].Name
	})

	assert.Equal(t, want, gotEdgeIngresses)
}

func loadFixtures[T any](t *testing.T, path string) []T {
	t.Helper()

	var objects []T
	if path == "" {
		return objects
	}

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(b), 1000)

	for {
		var object T
		if decoder.Decode(&object) != nil {
			break
		}

		objects = append(objects, object)
	}

	return objects
}
