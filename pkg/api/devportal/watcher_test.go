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

package devportal

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/scheme"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	listers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/listers/hub/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWatcher_Run(t *testing.T) {
	clientSet := hubkubemock.NewSimpleClientset()

	internalObjects := loadK8sObjects(t, clientSet, "./testdata/manifests/internal-portal.yaml")
	externalObjects := loadK8sObjects(t, clientSet, "./testdata/manifests/external-portal.yaml")

	portals, gateways, apis, collections, accesses := setupInformers(t, clientSet)

	wantPortals := []portal{
		{
			APIPortal: externalObjects.APIPortals["external-portal"],
			Gateway: gateway{
				APIGateway: externalObjects.APIGateways["external-gateway"],
				Collections: map[string]collection{
					"products": {
						APICollection: externalObjects.APICollections["products"],
						APIs: map[string]hubv1alpha1.API{
							"books@products-ns": externalObjects.APIs["books@products-ns"],
							"toys@products-ns":  externalObjects.APIs["toys@products-ns"],
						},
					},
				},
				APIs: map[string]hubv1alpha1.API{
					"search@default": externalObjects.APIs["search@default"],
				},
			},
		},
		{
			APIPortal: internalObjects.APIPortals["internal-portal"],
			Gateway: gateway{
				APIGateway:  internalObjects.APIGateways["internal-gateway"],
				Collections: map[string]collection{},
				APIs: map[string]hubv1alpha1.API{
					"accounting-reports@accounting-ns": internalObjects.APIs["accounting-reports@accounting-ns"],
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	handler := newUpdatableHandlerMock(t)
	handler.OnUpdateRaw(mock.AnythingOfType("[]devportal.portal")).
		Run(func(args mock.Arguments) {
			gotPortals := args.Get(0).([]portal)

			sort.Slice(gotPortals, func(i, j int) bool {
				return gotPortals[i].Name < gotPortals[j].Name
			})

			assert.Equal(t, wantPortals, gotPortals)
			cancel()
		}).
		TypedReturns(nil)

	w := setupWatcher(t, handler, portals, gateways, apis, collections, accesses)

	// Simulate k8s resource change.
	w.OnAdd(&hubv1alpha1.APIGateway{})

	w.Run(ctx)
}

func TestWatcher_OnAdd(t *testing.T) {
	clientSet := hubkubemock.NewSimpleClientset()
	portals, gateways, apis, collections, accesses := setupInformers(t, clientSet)

	tests := []struct {
		desc   string
		object runtime.Object
	}{
		{desc: "APIPortal", object: &hubv1alpha1.APIPortal{}},
		{desc: "APIGateway", object: &hubv1alpha1.APIGateway{}},
		{desc: "API", object: &hubv1alpha1.API{}},
		{desc: "APICollection", object: &hubv1alpha1.APICollection{}},
		{desc: "APIAccess", object: &hubv1alpha1.APIAccess{}},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			handler := newUpdatableHandlerMock(t)

			eventCh := make(chan struct{})
			handler.OnUpdateRaw(mock.Anything).
				Run(func(args mock.Arguments) {
					eventCh <- struct{}{}
				}).
				TypedReturns(nil)

			w := setupWatcher(t, handler, portals, gateways, apis, collections, accesses)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			go w.Run(ctx)

			w.OnAdd(test.object)

			select {
			case <-time.After(100 * time.Millisecond):
				require.Fail(t, "Timed out while waiting for update event")
			case <-eventCh:
			}
		})
	}
}

func TestWatcher_OnDelete(t *testing.T) {
	clientSet := hubkubemock.NewSimpleClientset()
	portals, gateways, apis, collections, accesses := setupInformers(t, clientSet)

	tests := []struct {
		desc   string
		object runtime.Object
	}{
		{desc: "APIPortal", object: &hubv1alpha1.APIPortal{}},
		{desc: "APIGateway", object: &hubv1alpha1.APIGateway{}},
		{desc: "API", object: &hubv1alpha1.API{}},
		{desc: "APICollection", object: &hubv1alpha1.APICollection{}},
		{desc: "APIAccess", object: &hubv1alpha1.APIAccess{}},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			handler := newUpdatableHandlerMock(t)

			eventCh := make(chan struct{})
			handler.OnUpdateRaw(mock.Anything).
				Run(func(args mock.Arguments) {
					eventCh <- struct{}{}
				}).
				TypedReturns(nil)

			w := setupWatcher(t, handler, portals, gateways, apis, collections, accesses)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			go w.Run(ctx)

			w.OnDelete(test.object)

			select {
			case <-time.After(100 * time.Millisecond):
				require.Fail(t, "Timed out while waiting for update event")
			case <-eventCh:
			}
		})
	}
}

func TestWatcher_OnUpdate(t *testing.T) {
	clientSet := hubkubemock.NewSimpleClientset()
	portals, gateways, apis, collections, accesses := setupInformers(t, clientSet)

	tests := []struct {
		desc       string
		oldObject  runtime.Object
		newObject  runtime.Object
		wantUpdate bool
	}{
		{
			desc:       "APIPortal: same hash and urls",
			oldObject:  &hubv1alpha1.APIPortal{Status: hubv1alpha1.APIPortalStatus{Hash: "v1", URLs: "http://url.com"}},
			newObject:  &hubv1alpha1.APIPortal{Status: hubv1alpha1.APIPortalStatus{Hash: "v1", URLs: "http://url.com"}},
			wantUpdate: false,
		},
		{
			desc:       "APIPortal: same hash and different urls",
			oldObject:  &hubv1alpha1.APIPortal{Status: hubv1alpha1.APIPortalStatus{Hash: "v1", URLs: "http://url.com"}},
			newObject:  &hubv1alpha1.APIPortal{Status: hubv1alpha1.APIPortalStatus{Hash: "v1", URLs: "http://url.com, http://other.com"}},
			wantUpdate: true,
		},
		{
			desc:       "APIPortal: different hash",
			oldObject:  &hubv1alpha1.APIPortal{Status: hubv1alpha1.APIPortalStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.APIPortal{Status: hubv1alpha1.APIPortalStatus{Hash: "v2"}},
			wantUpdate: true,
		},
		{
			desc:       "APIGateway: same hash and urls",
			oldObject:  &hubv1alpha1.APIGateway{Status: hubv1alpha1.APIGatewayStatus{Hash: "v1", URLs: "http://url.com"}},
			newObject:  &hubv1alpha1.APIGateway{Status: hubv1alpha1.APIGatewayStatus{Hash: "v1", URLs: "http://url.com"}},
			wantUpdate: false,
		},
		{
			desc:       "APIGateway: same hash and different urls",
			oldObject:  &hubv1alpha1.APIGateway{Status: hubv1alpha1.APIGatewayStatus{Hash: "v1", URLs: "http://url.com"}},
			newObject:  &hubv1alpha1.APIGateway{Status: hubv1alpha1.APIGatewayStatus{Hash: "v1", URLs: "http://url.com, http://other.com"}},
			wantUpdate: true,
		},
		{
			desc:       "APIGateway: different hash",
			oldObject:  &hubv1alpha1.APIGateway{Status: hubv1alpha1.APIGatewayStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.APIGateway{Status: hubv1alpha1.APIGatewayStatus{Hash: "v2"}},
			wantUpdate: true,
		},

		{
			desc:       "API: same hash",
			oldObject:  &hubv1alpha1.API{Status: hubv1alpha1.APIStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.API{Status: hubv1alpha1.APIStatus{Hash: "v1"}},
			wantUpdate: false,
		},
		{
			desc:       "API: different hash",
			oldObject:  &hubv1alpha1.API{Status: hubv1alpha1.APIStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.API{Status: hubv1alpha1.APIStatus{Hash: "v2"}},
			wantUpdate: true,
		},

		{
			desc:       "APICollection: same hash",
			oldObject:  &hubv1alpha1.APICollection{Status: hubv1alpha1.APICollectionStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.APICollection{Status: hubv1alpha1.APICollectionStatus{Hash: "v1"}},
			wantUpdate: false,
		},
		{
			desc:       "APICollection: different hash",
			oldObject:  &hubv1alpha1.APICollection{Status: hubv1alpha1.APICollectionStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.APICollection{Status: hubv1alpha1.APICollectionStatus{Hash: "v2"}},
			wantUpdate: true,
		},

		{
			desc:       "APIAccess: same hash",
			oldObject:  &hubv1alpha1.APIAccess{Status: hubv1alpha1.APIAccessStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.APIAccess{Status: hubv1alpha1.APIAccessStatus{Hash: "v1"}},
			wantUpdate: false,
		},
		{
			desc:       "APIAccess: different hash",
			oldObject:  &hubv1alpha1.APIAccess{Status: hubv1alpha1.APIAccessStatus{Hash: "v1"}},
			newObject:  &hubv1alpha1.APIAccess{Status: hubv1alpha1.APIAccessStatus{Hash: "v2"}},
			wantUpdate: true,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			handler := newUpdatableHandlerMock(t)

			eventCh := make(chan struct{})
			handler.OnUpdateRaw(mock.Anything).
				Run(func(args mock.Arguments) {
					eventCh <- struct{}{}
				}).
				TypedReturns(nil).
				Maybe()

			w := setupWatcher(t, handler, portals, gateways, apis, collections, accesses)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			go w.Run(ctx)

			w.OnUpdate(test.oldObject, test.newObject)

			select {
			case <-time.After(50 * time.Millisecond):
				if test.wantUpdate {
					require.Fail(t, "Timed out while waiting for update event")
				}
			case <-eventCh:
				if !test.wantUpdate {
					require.Fail(t, "Received an expected update event")
				}
			}
		})
	}
}

func loadK8sObjects(t *testing.T, clientSet *hubkubemock.Clientset, path string) *k8sObjects {
	t.Helper()

	objects := newK8sObjects()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	files := strings.Split(string(content), "---")

	for _, file := range files {
		if file == "\n" || file == "" {
			continue
		}

		decoder := scheme.Codecs.UniversalDeserializer()
		object, _, err := decoder.Decode([]byte(file), nil, nil)
		require.NoError(t, err)

		objects.Add(t, object)

		// clientSet.Tracker().Add uses an heuristic for finding the plural form of a resource. In the
		// case of `APIGateway` it outputs `apigatewaies` which doesn't match the real plural form `apigateway.
		// Therefore, preventing any ClientSet APIs and Listers from working. To alleviate the issue, such objects
		// must be manually created with the right GroupVersionResource.
		if object.GetObjectKind().GroupVersionKind().Kind == "APIGateway" {
			err = clientSet.Tracker().Create(schema.GroupVersionResource{
				Group:    "hub.traefik.io",
				Version:  "v1alpha1",
				Resource: "apigateways",
			}, object, "")
			require.NoError(t, err)
			continue
		}

		err = clientSet.Tracker().Add(object)
		require.NoError(t, err)
	}

	return objects
}

type k8sObjects struct {
	APIPortals     map[string]hubv1alpha1.APIPortal
	APIGateways    map[string]hubv1alpha1.APIGateway
	APICollections map[string]hubv1alpha1.APICollection
	APIs           map[string]hubv1alpha1.API
	APIAccesses    map[string]hubv1alpha1.APIAccess

	accessor meta.MetadataAccessor
}

func newK8sObjects() *k8sObjects {
	accessor := meta.NewAccessor()

	return &k8sObjects{
		accessor: accessor,

		APIPortals:     make(map[string]hubv1alpha1.APIPortal),
		APIGateways:    make(map[string]hubv1alpha1.APIGateway),
		APICollections: make(map[string]hubv1alpha1.APICollection),
		APIs:           make(map[string]hubv1alpha1.API),
		APIAccesses:    make(map[string]hubv1alpha1.APIAccess),
	}
}

func (o *k8sObjects) Add(t *testing.T, object runtime.Object) {
	t.Helper()

	kind, err := o.accessor.Kind(object)
	require.NoError(t, err)
	name, err := o.accessor.Name(object)
	require.NoError(t, err)
	namespace, err := o.accessor.Namespace(object)
	require.NoError(t, err)

	switch kind {
	case "APIPortal":
		o.APIPortals[name] = *object.(*hubv1alpha1.APIPortal)
	case "APIGateway":
		o.APIGateways[name] = *object.(*hubv1alpha1.APIGateway)
	case "APICollection":
		o.APICollections[name] = *object.(*hubv1alpha1.APICollection)
	case "API":
		o.APIs[name+"@"+namespace] = *object.(*hubv1alpha1.API)
	case "APIAccess":
		o.APIAccesses[name] = *object.(*hubv1alpha1.APIAccess)
	}
}

func setupInformers(t *testing.T, clientSet *hubkubemock.Clientset) (listers.APIPortalLister, listers.APIGatewayLister, listers.APILister, listers.APICollectionLister, listers.APIAccessLister) {
	t.Helper()

	hubInformer := hubinformer.NewSharedInformerFactory(clientSet, 5*time.Minute)

	portals := hubInformer.Hub().V1alpha1().APIPortals().Lister()
	gateways := hubInformer.Hub().V1alpha1().APIGateways().Lister()
	apis := hubInformer.Hub().V1alpha1().APIs().Lister()
	collections := hubInformer.Hub().V1alpha1().APICollections().Lister()
	accesses := hubInformer.Hub().V1alpha1().APIAccesses().Lister()

	ctx := context.Background()
	hubInformer.Start(ctx.Done())
	for _, ok := range hubInformer.WaitForCacheSync(ctx.Done()) {
		require.True(t, ok)
	}

	return portals, gateways, apis, collections, accesses
}

func setupWatcher(t *testing.T,
	handler UpdatableHandler,
	portals listers.APIPortalLister,
	gateways listers.APIGatewayLister,
	apis listers.APILister,
	collections listers.APICollectionLister,
	accesses listers.APIAccessLister,
) *Watcher {
	t.Helper()

	w := NewWatcher(handler, portals, gateways, apis, collections, accesses)
	w.debounceDelay = 0
	w.maxDebounceDelay = 0

	return w
}
