package edgeingress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

// PlatformClient for the EdgeIngress service.
type PlatformClient interface {
	GetEdgeIngresses(ctx context.Context) ([]EdgeIngress, error)
	GetCertificate(ctx context.Context) (Certificate, error)
}

// Watcher watches hub EdgeIngresses and sync them with the cluster.
type Watcher struct {
	interval          time.Duration
	client            PlatformClient
	hubClientSet      hubclientset.Interface
	hubInformer       hubinformer.SharedInformerFactory
	clientSet         clientset.Interface
	ingressClassName  string
	traefikEntryPoint string
}

// NewWatcher returns a new Watcher.
func NewWatcher(interval time.Duration, client PlatformClient, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory, clientSet clientset.Interface, ingressClassName, traefikEntryPoint string) (*Watcher, error) {
	if ingressClassName == "" {
		return nil, errors.New("ingressClassName must be set")
	}

	return &Watcher{
		interval:          interval,
		client:            client,
		hubClientSet:      hubClientSet,
		hubInformer:       hubInformer,
		clientSet:         clientSet,
		ingressClassName:  ingressClassName,
		traefikEntryPoint: traefikEntryPoint,
	}, nil
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
			if err = w.syncChildResources(ctx, clusterEdgeIng); err != nil {
				log.Error().Err(err).
					Str("name", clusterEdgeIng.Name).
					Str("namespace", clusterEdgeIng.Namespace).
					Msg("Sync child resources")
			}

			continue
		}

		if !found {
			eIng, err := w.createEdgeIngress(ctx, &platformEdgeIng)
			if err != nil {
				log.Error().Err(err).
					Str("name", platformEdgeIng.Name).
					Str("namespace", platformEdgeIng.Namespace).
					Msg("Creating EdgeIngress")
				continue
			}

			if err = w.syncChildResources(ctx, eIng); err != nil {
				log.Error().Err(err).
					Str("name", eIng.Name).
					Str("namespace", eIng.Namespace).
					Msg("Sync child resources when creating EdgeIngress")
			}

			continue
		}

		clusterEdgeIng.Spec = buildResourceSpec(&platformEdgeIng)
		eIng, err := w.updateEdgeIngress(ctx, clusterEdgeIng, &platformEdgeIng)
		if err != nil {
			log.Error().Err(err).
				Str("name", clusterEdgeIng.Name).
				Str("namespace", clusterEdgeIng.Namespace).
				Msg("Updating EdgeIngress")

			continue
		}

		if err = w.syncChildResources(ctx, eIng); err != nil {
			log.Error().Err(err).
				Str("name", eIng.Name).
				Str("namespace", eIng.Namespace).
				Msg("Sync child resources when updating EdgeIngress")
		}
	}

	w.cleanEdgeIngresses(ctx, clusterEdgeIngressByID)
}

func (w *Watcher) syncChildResources(ctx context.Context, edgeIng *hubv1alpha1.EdgeIngress) error {
	if err := w.upsertSecret(ctx, edgeIng); err != nil {
		return fmt.Errorf("upserting secret: %w", err)
	}

	if err := w.upsertIngress(ctx, edgeIng); err != nil {
		return fmt.Errorf("upserting ingress: %w", err)
	}

	return nil
}

func (w *Watcher) upsertSecret(ctx context.Context, edgeIng *hubv1alpha1.EdgeIngress) error {
	// This call can be optimized, since we are under pressure we chose the quickest solution.
	cert, err := w.client.GetCertificate(ctx)
	if err != nil {
		return err
	}

	secret, err := w.clientSet.CoreV1().Secrets(edgeIng.Namespace).Get(ctx, edgeIng.Name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get secret: %w", err)
	}

	if kerror.IsNotFound(err) {
		secret = buildSecret(edgeIng, &corev1.Secret{}, cert)
		_, err = w.clientSet.CoreV1().Secrets(edgeIng.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create secret: %w", err)
		}

		log.Debug().
			Str("name", secret.Name).
			Str("namespace", secret.Namespace).
			Msg("Secret created")

		return nil
	}

	if bytes.Equal(secret.Data["tls.crt"], cert.Certificate) {
		return nil
	}

	secret = buildSecret(edgeIng, secret, cert)
	_, updateErr := w.clientSet.CoreV1().Secrets(edgeIng.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if updateErr != nil {
		return fmt.Errorf("update secret: %w", updateErr)
	}

	log.Debug().
		Str("name", secret.Name).
		Str("namespace", secret.Namespace).
		Msg("Secret updated")

	return nil
}

func (w *Watcher) upsertIngress(ctx context.Context, edgeIng *hubv1alpha1.EdgeIngress) error {
	ing, err := w.clientSet.NetworkingV1().Ingresses(edgeIng.Namespace).Get(ctx, edgeIng.Name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		ing = buildIngress(edgeIng, &netv1.Ingress{}, w.ingressClassName, w.traefikEntryPoint)
		_, err = w.clientSet.NetworkingV1().Ingresses(edgeIng.Namespace).Create(ctx, ing, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ingress: %w", err)
		}

		log.Debug().
			Str("name", ing.Name).
			Str("namespace", ing.Namespace).
			Msg("Ingress created")

		return nil
	}

	ing = buildIngress(edgeIng, ing, w.ingressClassName, w.traefikEntryPoint)
	_, err = w.clientSet.NetworkingV1().Ingresses(edgeIng.Namespace).Update(ctx, ing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ingress: %w", err)
	}

	log.Debug().
		Str("name", ing.Name).
		Str("namespace", ing.Namespace).
		Msg("Ingress updated")

	return nil
}

func (w *Watcher) createEdgeIngress(ctx context.Context, edgeIng *EdgeIngress) (*hubv1alpha1.EdgeIngress, error) {
	obj, err := edgeIng.Resource()
	if err != nil {
		return nil, fmt.Errorf("build EdgeIngress resource: %w", err)
	}

	ctxCreate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	obj, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(obj.Namespace).Create(ctxCreate, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Str("namespace", obj.Namespace).
		Msg("EdgeIngress created")

	return obj, nil
}

func (w *Watcher) updateEdgeIngress(ctx context.Context, oldEdgeIng *hubv1alpha1.EdgeIngress, newEdgeIng *EdgeIngress) (*hubv1alpha1.EdgeIngress, error) {
	ctxUpdate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	obj, err := newEdgeIng.Resource()
	if err != nil {
		return nil, fmt.Errorf("build EdgeIngress resource: %w", err)
	}

	oldEdgeIng.Spec = obj.Spec
	oldEdgeIng.Status = obj.Status

	obj, err = w.hubClientSet.HubV1alpha1().EdgeIngresses(obj.Namespace).Update(ctxUpdate, oldEdgeIng, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("updating EdgeIngress: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Str("namespace", obj.Namespace).
		Msg("EdgeIngress updated")

	return obj, nil
}

func (w *Watcher) cleanEdgeIngresses(ctx context.Context, edgeIngs map[string]*hubv1alpha1.EdgeIngress) {
	for _, edgeIng := range edgeIngs {
		ctxDelete, cancel := context.WithTimeout(ctx, 5*time.Second)

		// Foreground propagation allow us to delete all ingresses owned by the edgeIngress.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().EdgeIngresses(edgeIng.Namespace).Delete(ctxDelete, edgeIng.Name, opts)
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
			Name: edgeIng.Service.Name,
			Port: edgeIng.Service.Port,
		},
	}

	if edgeIng.ACP != nil {
		spec.ACP = &hubv1alpha1.EdgeIngressACP{
			Name: edgeIng.ACP.Name,
		}
	}

	return spec
}

func buildIngress(edgeIng *hubv1alpha1.EdgeIngress, ing *netv1.Ingress, ingressClassName, entryPoint string) *netv1.Ingress {
	annotations := map[string]string{
		"traefik.ingress.kubernetes.io/router.tls":         "true",
		"traefik.ingress.kubernetes.io/router.entrypoints": entryPoint,
	}
	if edgeIng.Spec.ACP != nil && edgeIng.Spec.ACP.Name != "" {
		annotations[reviewer.AnnotationHubAuth] = edgeIng.Spec.ACP.Name
	}

	ing.ObjectMeta = metav1.ObjectMeta{
		Name:        edgeIng.Name,
		Namespace:   edgeIng.Namespace,
		Annotations: annotations,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
		// Set OwnerReference allow us to delete ingresses owned by an edgeIngress.
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "EdgeIngress",
				Name:       edgeIng.Name,
				UID:        edgeIng.UID,
			},
		},
	}

	pathType := netv1.PathTypePrefix
	ing.Spec = netv1.IngressSpec{
		IngressClassName: pointer.StringPtr(ingressClassName),
		TLS: []netv1.IngressTLS{
			{
				Hosts:      []string{edgeIng.Status.Domain},
				SecretName: edgeIng.Name,
			},
		},
		Rules: []netv1.IngressRule{
			{
				Host: edgeIng.Status.Domain,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &pathType,
								Backend: netv1.IngressBackend{
									Service: &netv1.IngressServiceBackend{
										Name: edgeIng.Spec.Service.Name,
										Port: netv1.ServiceBackendPort{
											Number: int32(edgeIng.Spec.Service.Port),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return ing
}

func buildSecret(edgeIng *hubv1alpha1.EdgeIngress, secret *corev1.Secret, cert Certificate) *corev1.Secret {
	secret.ObjectMeta = metav1.ObjectMeta{
		Name:      edgeIng.Name,
		Namespace: edgeIng.Namespace,
		Annotations: map[string]string{
			"app.kubernetes.io/managed-by": "traefik-hub",
		},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "EdgeIngress",
				Name:       edgeIng.Name,
				UID:        edgeIng.UID,
			},
		},
	}
	secret.Type = corev1.SecretTypeTLS
	secret.Data = map[string][]byte{
		"tls.crt": cert.Certificate,
		"tls.key": cert.PrivateKey,
	}

	return secret
}
