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
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	hublistersv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/listers/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"
)

// Mandatory to be able to parse traefik.containo.us/v1alpha1 and traefik.io/v1alpha1 resources.
func init() {
	if err := hubv1alpha1.AddToScheme(kscheme.Scheme); err != nil {
		panic(err)
	}
}

func TestWatcher_Run(t *testing.T) {
	internalObjects := kube.LoadK8sObjects(t, "./testdata/manifests/internal-portal.yaml")
	internalK8sObjects := newK8sObjects(t, internalObjects)

	externalObjects := kube.LoadK8sObjects(t, "./testdata/manifests/external-portal.yaml")
	externalK8sObjects := newK8sObjects(t, externalObjects)

	clientSet := kube.NewFakeHubClientset(append(internalObjects, externalObjects...)...)

	portals, gateways, apis, collections, accesses := setupInformers(t, clientSet)

	wantPortals := []portal{
		{
			APIPortal: externalK8sObjects.APIPortals["external-portal"],
			Gateway: gateway{
				APIGateway: externalK8sObjects.APIGateways["external-gateway"],
				Collections: map[string]collection{
					"products": {
						APICollection: externalK8sObjects.APICollections["products"],
						APIs: map[string]api{
							"books@products-ns": {API: externalK8sObjects.APIs["books@products-ns"], authorizedGroups: []string{"supplier"}},
							"toys@products-ns":  {API: externalK8sObjects.APIs["toys@products-ns"], authorizedGroups: []string{"supplier"}},
						},
						authorizedGroups: []string{"supplier"},
					},
				},
				APIs: map[string]api{
					"search@default": {API: externalK8sObjects.APIs["search@default"], authorizedGroups: []string{"consumer"}},
				},
			},
		},
		{
			APIPortal: internalK8sObjects.APIPortals["internal-portal"],
			Gateway: gateway{
				APIGateway:  internalK8sObjects.APIGateways["internal-gateway"],
				Collections: map[string]collection{},
				APIs: map[string]api{
					"accounting-reports@accounting-ns": {API: internalK8sObjects.APIs["accounting-reports@accounting-ns"], authorizedGroups: []string{"accounting-team"}},
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
	clientSet := hubfake.NewSimpleClientset()
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
	clientSet := hubfake.NewSimpleClientset()
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
	clientSet := hubfake.NewSimpleClientset()
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

type k8sObjects struct {
	APIPortals     map[string]hubv1alpha1.APIPortal
	APIGateways    map[string]hubv1alpha1.APIGateway
	APICollections map[string]hubv1alpha1.APICollection
	APIs           map[string]hubv1alpha1.API
	APIAccesses    map[string]hubv1alpha1.APIAccess
}

func newK8sObjects(t *testing.T, objects []runtime.Object) *k8sObjects {
	t.Helper()

	accessor := meta.NewAccessor()

	o := &k8sObjects{
		APIPortals:     make(map[string]hubv1alpha1.APIPortal),
		APIGateways:    make(map[string]hubv1alpha1.APIGateway),
		APICollections: make(map[string]hubv1alpha1.APICollection),
		APIs:           make(map[string]hubv1alpha1.API),
		APIAccesses:    make(map[string]hubv1alpha1.APIAccess),
	}

	for _, object := range objects {
		name, err := accessor.Name(object)
		require.NoError(t, err)

		namespace, err := accessor.Namespace(object)
		require.NoError(t, err)

		switch obj := object.(type) {
		case *hubv1alpha1.APIPortal:
			o.APIPortals[name] = *obj
		case *hubv1alpha1.APIGateway:
			o.APIGateways[name] = *obj
		case *hubv1alpha1.APICollection:
			o.APICollections[name] = *obj
		case *hubv1alpha1.API:
			o.APIs[name+"@"+namespace] = *obj
		case *hubv1alpha1.APIAccess:
			o.APIAccesses[name] = *obj
		default:
			t.Fatal("unknown type", obj)
		}
	}

	return o
}

func setupInformers(t *testing.T, clientSet *hubfake.Clientset) (hublistersv1alpha1.APIPortalLister, hublistersv1alpha1.APIGatewayLister, hublistersv1alpha1.APILister, hublistersv1alpha1.APICollectionLister, hublistersv1alpha1.APIAccessLister) {
	t.Helper()

	hubInformer := hubinformers.NewSharedInformerFactory(clientSet, 5*time.Minute)

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
	portals hublistersv1alpha1.APIPortalLister,
	gateways hublistersv1alpha1.APIGatewayLister,
	apis hublistersv1alpha1.APILister,
	collections hublistersv1alpha1.APICollectionLister,
	accesses hublistersv1alpha1.APIAccessLister,
) *Watcher {
	t.Helper()

	w := NewWatcher(handler, portals, gateways, apis, collections, accesses)
	w.debounceDelay = 0
	w.maxDebounceDelay = 0

	return w
}
