package acme

import "k8s.io/client-go/tools/cache"

// DelayedResourceEventHandler is a ResourceEventHandler implementation which delays the call to the wrapped handler until the sync channel is closed.
type DelayedResourceEventHandler struct {
	SyncChan chan struct{}
	Handler  cache.ResourceEventHandler
}

// OnAdd blocks until the sync channel is closed and calls the wrapped handler.
func (r DelayedResourceEventHandler) OnAdd(obj interface{}) {
	<-r.SyncChan
	r.Handler.OnAdd(obj)
}

// OnUpdate blocks until the sync channel is closed and calls the wrapped handler.
func (r DelayedResourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	<-r.SyncChan
	r.Handler.OnUpdate(oldObj, newObj)
}

// OnDelete blocks until the sync channel is closed and calls the wrapped handler.
func (r DelayedResourceEventHandler) OnDelete(obj interface{}) {
	<-r.SyncChan
	r.Handler.OnDelete(obj)
}
