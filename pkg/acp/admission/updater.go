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

package admission

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	"github.com/traefik/hub-agent-kubernetes/pkg/kubevers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
)

// IngressUpdater handles ingress updates when ACP configurations are modified.
type IngressUpdater struct {
	informer  informers.SharedInformerFactory
	clientSet clientset.Interface

	cancelUpd map[string]context.CancelFunc

	polNameCh chan string

	supportsNetV1Ingresses bool
}

// NewIngressUpdater return a new IngressUpdater.
func NewIngressUpdater(informer informers.SharedInformerFactory, clientSet clientset.Interface, kubeVersion string) *IngressUpdater {
	return &IngressUpdater{
		informer:               informer,
		clientSet:              clientSet,
		cancelUpd:              map[string]context.CancelFunc{},
		polNameCh:              make(chan string),
		supportsNetV1Ingresses: kubevers.SupportsNetV1Ingresses(kubeVersion),
	}
}

// Run runs the IngressUpdater control loop, updating ingress resources when needed.
func (u *IngressUpdater) Run(ctx context.Context) {
	for {
		select {
		case polName := <-u.polNameCh:
			if cancel, ok := u.cancelUpd[polName]; ok {
				cancel()
				delete(u.cancelUpd, polName)
			}

			ctxUpd, cancel := context.WithCancel(ctx)
			u.cancelUpd[polName] = cancel

			go func(polName string) {
				err := u.updateIngresses(ctxUpd, polName)
				if err != nil {
					log.Error().Err(err).Str("acp_name", polName).Msg("Unable to update ingresses")
				}
			}(polName)

		case <-ctx.Done():
			return
		}
	}
}

// Update notifies the IngressUpdater control loop that it should update ingresses referencing the given ACP if they had
// a header-related configuration change.
func (u *IngressUpdater) Update(polName string) {
	u.polNameCh <- polName
}

func (u *IngressUpdater) updateIngresses(ctx context.Context, polName string) error {
	if !u.supportsNetV1Ingresses {
		return u.updateV1beta1Ingresses(ctx, polName)
	}

	return u.updateV1Ingresses(ctx, polName)
}

func (u *IngressUpdater) updateV1Ingresses(ctx context.Context, polName string) error {
	ingList, err := u.informer.Networking().V1().Ingresses().Lister().List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}

	log.Debug().Int("ingress_number", len(ingList)).Msg("Updating ingresses")

	for _, ing := range ingList {
		// Don't continue if the context was canceled to prevent being spammed
		// with context canceled errors on every request we would send otherwise.
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		ok := shouldUpdate(ing.Annotations[reviewer.AnnotationHubAuth], polName)
		if err != nil {
			log.Error().Err(err).Str("ingress_name", ing.Name).Str("ingress_namespace", ing.Namespace).Msg("Unable to determine if ingress should be updated")
			continue
		}
		if !ok {
			continue
		}

		_, err = u.clientSet.NetworkingV1().Ingresses(ing.Namespace).Update(ctx, ing, metav1.UpdateOptions{FieldManager: "hub-auth"})
		if err != nil {
			log.Error().Err(err).Str("ingress_name", ing.Name).Str("ingress_namespace", ing.Namespace).Msg("Unable to update ingress")
			continue
		}
	}
	return nil
}

func (u *IngressUpdater) updateV1beta1Ingresses(ctx context.Context, polName string) error {
	// As the minimum supported version is 1.14, we don't need to support the extension group.
	ingList, err := u.informer.Networking().V1beta1().Ingresses().Lister().List(labels.Everything())
	if err != nil {
		return fmt.Errorf("list legacy ingresses: %w", err)
	}

	log.Debug().Int("ingress_number", len(ingList)).Msg("Updating legacy ingresses")

	for _, ing := range ingList {
		// Don't continue if the context was canceled to prevent being spammed
		// with context canceled errors on every request we would send otherwise.
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		ok := shouldUpdate(ing.Annotations[reviewer.AnnotationHubAuth], polName)
		if err != nil {
			log.Error().Err(err).Str("ingress_name", ing.Name).Str("ingress_namespace", ing.Namespace).Msg("Unable to determine if legacy ingress should be updated")
			continue
		}
		if !ok {
			continue
		}

		_, err = u.clientSet.NetworkingV1beta1().Ingresses(ing.Namespace).Update(ctx, ing, metav1.UpdateOptions{FieldManager: "hub-auth"})
		if err != nil {
			log.Error().Err(err).Str("ingress_name", ing.Name).Str("ingress_namespace", ing.Namespace).Msg("Unable to update legacy ingress")
			continue
		}
	}
	return nil
}

func shouldUpdate(hubAuthAnno, polName string) bool {
	if hubAuthAnno == "" {
		return false
	}

	if hubAuthAnno != polName {
		return false
	}

	return true
}
