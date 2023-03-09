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
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
)

// PlatformClient for the API service.
type PlatformClient interface {
	GetPortals(ctx context.Context) ([]Portal, error)
	GetWildcardCertificate(ctx context.Context) (edgeingress.Certificate, error)
	GetCertificateByDomains(ctx context.Context, domains []string) (edgeingress.Certificate, error)
	GetGateways(ctx context.Context) ([]Gateway, error)
	GetAPIs(ctx context.Context) ([]API, error)
	GetCollections(ctx context.Context) ([]Collection, error)
}

// WatcherPortalConfig holds the portal watcher configuration.
type WatcherPortalConfig struct {
	IngressClassName        string
	AgentNamespace          string
	TraefikAPIEntryPoint    string
	TraefikTunnelEntryPoint string
	DevPortalServiceName    string
	DevPortalPort           int

	PortalSyncInterval time.Duration
}

// WatcherPortal watches hub portals and sync them with the cluster.
type WatcherPortal struct {
	config *WatcherPortalConfig

	platform PlatformClient

	kubeClientSet clientset.Interface
	kubeInformer  informers.SharedInformerFactory

	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcherPortal returns a new WatcherPortal.
func NewWatcherPortal(client PlatformClient, kubeClientSet clientset.Interface, kubeInformer informers.SharedInformerFactory, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, config *WatcherPortalConfig) *WatcherPortal {
	return &WatcherPortal{
		config: config,

		platform: client,

		kubeClientSet: kubeClientSet,
		kubeInformer:  kubeInformer,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs WatcherPortal.
func (w *WatcherPortal) Run(ctx context.Context) {
	t := time.NewTicker(w.config.PortalSyncInterval)
	defer t.Stop()

	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
	w.syncPortals(ctxSync)
	cancel()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping API watcher")
			return

		case <-t.C:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			w.syncPortals(ctxSync)
			cancel()
		}
	}
}

func (w *WatcherPortal) syncPortals(ctx context.Context) {
	platformPortals, err := w.platform.GetPortals(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch APIPortals")
		return
	}

	clusterPortals, err := w.hubInformer.Hub().V1alpha1().APIPortals().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain APIPortals")
		return
	}

	portalsByName := map[string]*hubv1alpha1.APIPortal{}
	for _, portal := range clusterPortals {
		portalsByName[portal.Name] = portal
	}

	for _, portal := range platformPortals {
		platformPortal := portal

		clusterPortal, found := portalsByName[platformPortal.Name]

		// Portals that will remain in the map will be deleted.
		delete(portalsByName, platformPortal.Name)

		if !found {
			if err = w.createPortal(ctx, &platformPortal); err != nil {
				log.Error().Err(err).
					Str("name", platformPortal.Name).
					Msg("Unable to create APIPortal")
			}
			continue
		}

		if err = w.updatePortal(ctx, clusterPortal, &platformPortal); err != nil {
			log.Error().Err(err).
				Str("name", platformPortal.Name).
				Msg("Unable to update APIPortal")
		}
	}

	w.cleanPortals(ctx, portalsByName)
}

func (w *WatcherPortal) createPortal(ctx context.Context, portal *Portal) error {
	obj, err := portal.Resource()
	if err != nil {
		return fmt.Errorf("build APIPortal resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().APIPortals().Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating APIPortal: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("APIPortal created")

	return w.syncChildResources(ctx, obj)
}

func (w *WatcherPortal) updatePortal(ctx context.Context, oldPortal *hubv1alpha1.APIPortal, newPortal *Portal) error {
	obj, err := newPortal.Resource()
	if err != nil {
		return fmt.Errorf("build APIPortal resource: %w", err)
	}

	obj.ObjectMeta = oldPortal.ObjectMeta

	if obj.Status.Version != oldPortal.Status.Version {
		obj, err = w.hubClientSet.HubV1alpha1().APIPortals().Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating APIPortal: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("APIPortal updated")
	}

	return w.syncChildResources(ctx, obj)
}

func (w *WatcherPortal) cleanPortals(ctx context.Context, portals map[string]*hubv1alpha1.APIPortal) {
	for _, portal := range portals {
		// Foreground propagation allow us to delete all resources owned by the APIPortal.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().APIPortals().Delete(ctx, portal.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete APIPortal")

			continue
		}

		log.Debug().
			Str("name", portal.Name).
			Msg("APIPortal deleted")
	}
}

func (w *WatcherPortal) syncChildResources(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	if err := w.upsertPortalEdgeIngress(ctx, portal); err != nil {
		return fmt.Errorf("upsert portal edge ingress: %w", err)
	}

	return nil
}

func (w *WatcherPortal) upsertPortalEdgeIngress(ctx context.Context, portal *hubv1alpha1.APIPortal) error {
	ingName, err := getEdgeIngressPortalName(portal.Name)
	if err != nil {
		return fmt.Errorf("get edge ingress name: %w", err)
	}

	clusterIng, err := w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Get(ctx, ingName, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get edge ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		ing := &hubv1alpha1.EdgeIngress{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "EdgeIngress",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingName,
				Namespace: w.config.AgentNamespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "traefik-hub",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: portal.APIVersion,
						Kind:       portal.Kind,
						Name:       portal.Name,
						UID:        portal.UID,
					},
				},
			},
			Spec: hubv1alpha1.EdgeIngressSpec{
				Service: hubv1alpha1.EdgeIngressService{
					Name: w.config.DevPortalServiceName,
					Port: w.config.DevPortalPort,
				},
				CustomDomains: portal.Spec.CustomDomains,
			},
		}

		clusterIng, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(w.config.AgentNamespace).Create(ctx, ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create edge ingress: %w", err)
		}
	}

	// Set the APIPortal HubDomain with the domain obtained from the EdgeIngress.
	patch := []byte(fmt.Sprintf(`[
		{ "op": "replace", "path": "/status/hubDomain", "value": %q }
	]`, clusterIng.Status.Domain))

	if _, err = w.hubClientSet.HubV1alpha1().APIPortals().Patch(ctx, portal.Name, ktypes.JSONPatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch APIPortal: %w", err)
	}

	return nil
}

// getEdgeIngressPortalName compute the edge ingress portal name from the portal name.
// The name follow this format: {portal-name}-{hash(portal-name)}-portal
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getEdgeIngressPortalName(portalName string) (string, error) {
	h, err := hash(portalName)
	if err != nil {
		return "", err
	}

	// EdgeIngresses generate Ingresses with the same name. Therefore, to prevent any conflicts between the portal
	// ingress and the portal ingresses the term "-portal" must be added as a suffix.
	return fmt.Sprintf("%s-%d-portal", portalName, h), nil
}

func hash(name string) (uint32, error) {
	h := fnv.New32()

	if _, err := h.Write([]byte(name)); err != nil {
		return 0, fmt.Errorf("generate hash: %w", err)
	}

	return h.Sum32(), nil
}
