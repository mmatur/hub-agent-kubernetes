/*
Copyright (C) 2022 Traefik Labs

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
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

// ingressClass is an internal representation of either a netv1.IngressClass,
// netv1beta1.IngressClass or a hubv1alpha1.IngressClass.
type ingressClass struct {
	Name       string
	Controller string
	IsDefault  bool
}

const annotationDefaultIngressClass = "ingressclass.kubernetes.io/is-default-class"

// Supported ingress controller types.
const (
	ControllerTypeNginxCommunity = "k8s.io/ingress-nginx"
	ControllerTypeTraefik        = "traefik.io/ingress-controller"
)

// Watcher watches for IngressClass resources, maintaining a local cache of these resources,
// updated as they are created, modified or deleted.
// It watches for netv1.IngressClass, netv1beta1.IngressClass and hubv1alpha1.IngressClass.
type Watcher struct {
	mu             sync.RWMutex
	ingressClasses map[ktypes.UID]ingressClass
}

// NewWatcher creates a new Watcher to track IngressClass resources.
func NewWatcher() *Watcher {
	return &Watcher{
		ingressClasses: make(map[ktypes.UID]ingressClass),
	}
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnAdd(obj interface{}) {
	w.upsert(obj)
}

// OnUpdate implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnUpdate(_, newObj interface{}) {
	w.upsert(newObj)
}

// OnDelete implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnDelete(obj interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch v := obj.(type) {
	case *netv1.IngressClass:
		delete(w.ingressClasses, v.ObjectMeta.UID)
	case *netv1beta1.IngressClass:
		delete(w.ingressClasses, v.ObjectMeta.UID)
	case *hubv1alpha1.IngressClass:
		delete(w.ingressClasses, v.ObjectMeta.UID)
	default:
		log.Error().
			Str("component", "ingress_class_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received delete event of unknown type")
	}
}

func (w *Watcher) upsert(obj interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch v := obj.(type) {
	case *netv1.IngressClass:
		w.ingressClasses[v.ObjectMeta.UID] = ingressClass{
			Name:       v.ObjectMeta.Name,
			Controller: v.Spec.Controller,
			IsDefault:  v.ObjectMeta.Annotations[annotationDefaultIngressClass] == "true",
		}
	case *netv1beta1.IngressClass:
		w.ingressClasses[v.ObjectMeta.UID] = ingressClass{
			Name:       v.ObjectMeta.Name,
			Controller: v.Spec.Controller,
			IsDefault:  v.ObjectMeta.Annotations[annotationDefaultIngressClass] == "true",
		}
	case *hubv1alpha1.IngressClass:
		w.ingressClasses[v.ObjectMeta.UID] = ingressClass{
			Name:       v.ObjectMeta.Name,
			Controller: v.Spec.Controller,
			IsDefault:  v.ObjectMeta.Annotations[annotationDefaultIngressClass] == "true",
		}
	default:
		log.Error().
			Str("component", "ingress_class_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received upsert event of unknown type")
	}
}

// GetController returns the controller of the IngressClass matching the given name. If no IngressClass
// is found, an empty string is returned.
func (w *Watcher) GetController(name string) (string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, class := range w.ingressClasses {
		if class.Name == name {
			return class.Controller, nil
		}
	}

	return "", fmt.Errorf("IngressClass %q not found", name)
}

// GetDefaultController returns the controller of the IngressClass that is noted as default.
// If no IngressClass is noted as default, an empty string is returned.
// If multiple IngressClasses are marked as default, an error is returned instead.
func (w *Watcher) GetDefaultController() (string, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var ctrlr string
	for _, ic := range w.ingressClasses {
		if ic.IsDefault {
			if ctrlr == "" {
				ctrlr = ic.Controller
				continue
			}
			return "", errors.New("multiple default ingress classes found")
		}
	}

	return ctrlr, nil
}
