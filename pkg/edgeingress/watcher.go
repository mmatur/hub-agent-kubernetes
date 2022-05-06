package edgeingress

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Client for the EdgeIngress service.
type Client interface {
	GetEdgeIngresses(ctx context.Context) ([]EdgeIngress, error)
}

// Watcher watches hub EdgeIngresses and sync them with the cluster.
type Watcher struct {
	interval     time.Duration
	client       Client
	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcher returns a new Watcher.
func NewWatcher(interval time.Duration, client Client, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory) *Watcher {
	return &Watcher{
		interval:     interval,
		client:       client,
		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs Watcher.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping EdgeIngress watcher")
			return
		case <-t.C:
			w.syncEdgeIngresses(ctx)
		}
	}
}

func (w *Watcher) syncEdgeIngresses(ctx context.Context) {
	ctxFetch, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	platformEdgeIngresses, err := w.client.GetEdgeIngresses(ctxFetch)
	if err != nil {
		log.Error().Err(err).Msg("Fetching EdgeIngresses")
		return
	}

	clusterEdgeIngresses, err := w.hubInformer.Hub().V1alpha1().EdgeIngresses().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Listing EdgeIngresses")
		return
	}

	clusterEdgeIngressByID := map[string]*hubv1alpha1.EdgeIngress{}
	for _, edgeIng := range clusterEdgeIngresses {
		clusterEdgeIngressByID[edgeIng.Name+"@"+edgeIng.Namespace] = edgeIng
	}

	for _, p := range platformEdgeIngresses {
		platformEdgeIng := p

		clusterEdgeIng, found := clusterEdgeIngressByID[platformEdgeIng.Name+"@"+platformEdgeIng.Namespace]
		// We delete the policy from the map, since we use this map to delete unused policies.
		delete(clusterEdgeIngressByID, platformEdgeIng.Name+"@"+platformEdgeIng.Namespace)

		if found && !needUpdate(buildResourceSpec(&platformEdgeIng), clusterEdgeIng.Spec) {
			continue
		}

		if !found {
			if err = w.createEdgeIngress(ctx, &platformEdgeIng); err != nil {
				log.Error().Err(err).
					Str("name", platformEdgeIng.Name).
					Str("namespace", platformEdgeIng.Namespace).
					Msg("Creating EdgeIngress")
			}
			continue
		}

		clusterEdgeIng.Spec = buildResourceSpec(&platformEdgeIng)
		if err = w.updateEdgeIngress(ctx, clusterEdgeIng, &platformEdgeIng); err != nil {
			log.Error().Err(err).
				Str("name", clusterEdgeIng.Name).
				Str("namespace", clusterEdgeIng.Namespace).
				Msg("Updating EdgeIngress")
		}
	}

	w.cleanEdgeIngresses(ctx, clusterEdgeIngressByID)
}

func (w *Watcher) createEdgeIngress(ctx context.Context, edgeIng *EdgeIngress) error {
	obj, err := edgeIng.Resource()
	if err != nil {
		return fmt.Errorf("build EdgeIngress resource: %w", err)
	}

	ctxCreate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = w.hubClientSet.HubV1alpha1().
		EdgeIngresses(obj.Namespace).
		Create(ctxCreate, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Str("namespace", obj.Namespace).
		Msg("EdgeIngress created")

	return nil
}

func (w *Watcher) updateEdgeIngress(ctx context.Context, oldEdgeIng *hubv1alpha1.EdgeIngress, newEdgeIng *EdgeIngress) error {
	ctxUpdate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	obj, err := newEdgeIng.Resource()
	if err != nil {
		return fmt.Errorf("build EdgeIngress resource: %w", err)
	}

	oldEdgeIng.Spec = obj.Spec
	oldEdgeIng.Status = obj.Status

	_, err = w.hubClientSet.HubV1alpha1().
		EdgeIngresses(obj.Namespace).
		Update(ctxUpdate, oldEdgeIng, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Str("namespace", obj.Namespace).
		Msg("EdgeIngress updated")

	return nil
}

func (w *Watcher) cleanEdgeIngresses(ctx context.Context, edgeIngs map[string]*hubv1alpha1.EdgeIngress) {
	for _, edgeIng := range edgeIngs {
		ctxDelete, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := w.hubClientSet.HubV1alpha1().
			EdgeIngresses(edgeIng.Namespace).
			Delete(ctxDelete, edgeIng.Name, metav1.DeleteOptions{})
		if err != nil {
			log.Error().Err(err).Msg("Deleting EdgeIngress")
			cancel()
			continue
		}
		cancel()

		log.Debug().
			Str("name", edgeIng.Name).
			Str("namespace", edgeIng.Namespace).
			Msg("EdgeIngress deleted")
	}
}

func needUpdate(a, b hubv1alpha1.EdgeIngressSpec) bool {
	return !reflect.DeepEqual(a, b)
}

func buildResourceSpec(edgeIng *EdgeIngress) hubv1alpha1.EdgeIngressSpec {
	spec := hubv1alpha1.EdgeIngressSpec{
		Service: hubv1alpha1.EdgeIngressService{
			Name: edgeIng.ServiceName,
			Port: edgeIng.ServicePort,
		},
	}

	if edgeIng.ACPName != "" {
		spec.ACP = &hubv1alpha1.EdgeIngressACP{
			Name:      edgeIng.ACPName,
			Namespace: edgeIng.ACPNamespace,
		}
	}

	return spec
}
