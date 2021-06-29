package admission

import (
	"fmt"
	"reflect"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Updatable represents a object that is updatable.
type Updatable interface {
	Update(polName string)
}

// EventHandler watches ACP resources and calls its set Updatable when they are modified.
type EventHandler struct {
	listener Updatable
}

// NewEventHandler returns a new event handler meant to listen for ACP changes. It calls the given Updatable when an ACP is modified.
func NewEventHandler(listener Updatable) *EventHandler {
	return &EventHandler{
		listener: listener,
	}
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *EventHandler) OnAdd(obj interface{}) {
	v, ok := obj.(*hubv1alpha1.AccessControlPolicy)
	if !ok {
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received add event of unknown type")
		return
	}

	polName := canonicalName(v.ObjectMeta.Name, v.ObjectMeta.Namespace)
	w.listener.Update(polName)
}

// OnUpdate implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *EventHandler) OnUpdate(oldObj, newObj interface{}) {
	newACP, ok := newObj.(*hubv1alpha1.AccessControlPolicy)
	if !ok {
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", newObj)).
			Msg("Received update event of unknown type (old)")
		return
	}

	oldACP, ok := oldObj.(*hubv1alpha1.AccessControlPolicy)
	if !ok {
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", oldObj)).
			Msg("Received update event of unknown type (new)")
		return
	}

	if !headersChanged(oldACP.Spec, newACP.Spec) {
		return
	}

	polName := canonicalName(newACP.ObjectMeta.Name, newACP.ObjectMeta.Namespace)
	w.listener.Update(polName)
}

// OnDelete implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *EventHandler) OnDelete(obj interface{}) {
	v, ok := obj.(*hubv1alpha1.AccessControlPolicy)
	if !ok {
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received delete event of unknown type")
		return
	}

	polName := canonicalName(v.ObjectMeta.Name, v.ObjectMeta.Namespace)
	w.listener.Update(polName)
}

func canonicalName(name, ns string) string {
	if ns == "" {
		ns = metav1.NamespaceDefault
	}

	return name + "@" + ns
}

func headersChanged(oldCfg, newCfg hubv1alpha1.AccessControlPolicySpec) bool {
	switch {
	case newCfg.JWT != nil:
		if oldCfg.JWT == nil {
			return true
		}

		return !reflect.DeepEqual(oldCfg.JWT.ForwardHeaders, newCfg.JWT.ForwardHeaders) ||
			oldCfg.JWT.StripAuthorizationHeader != newCfg.JWT.StripAuthorizationHeader

	case newCfg.BasicAuth != nil:
		if oldCfg.BasicAuth == nil {
			return true
		}

		return newCfg.BasicAuth.ForwardUsernameHeader != oldCfg.BasicAuth.ForwardUsernameHeader ||
			newCfg.BasicAuth.StripAuthorizationHeader != oldCfg.BasicAuth.StripAuthorizationHeader

	case newCfg.DigestAuth != nil:
		if oldCfg.DigestAuth == nil {
			return true
		}

		return newCfg.DigestAuth.ForwardUsernameHeader != oldCfg.DigestAuth.ForwardUsernameHeader ||
			newCfg.DigestAuth.StripAuthorizationHeader != oldCfg.DigestAuth.StripAuthorizationHeader

	default:
		return false
	}
}
