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
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/typed/traefik/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	kinformers "k8s.io/client-go/informers"
	kclientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

const (
	hubDomainSecretName          = "hub-certificate"
	customDomainSecretNamePrefix = "hub-certificate-custom-domains"
)

// WatcherGatewayConfig holds the watcher gateway configuration.
type WatcherGatewayConfig struct {
	IngressClassName        string
	AgentNamespace          string
	TraefikAPIEntryPoint    string
	TraefikTunnelEntryPoint string

	GatewaySyncInterval time.Duration
	CertSyncInterval    time.Duration
	CertRetryInterval   time.Duration
}

// WatcherGateway watches hub gateways and sync them with the cluster.
type WatcherGateway struct {
	config *WatcherGatewayConfig

	wildCardCertMu sync.RWMutex
	wildCardCert   edgeingress.Certificate

	platform PlatformClient

	kubeClientSet kclientset.Interface
	kubeInformer  kinformers.SharedInformerFactory

	hubClientSet hubclientset.Interface
	hubInformer  hubinformers.SharedInformerFactory

	traefikClientSet v1alpha1.TraefikV1alpha1Interface
}

// NewWatcherGateway returns a new WatcherGateway.
func NewWatcherGateway(client PlatformClient, kubeClientSet kclientset.Interface, kubeInformer kinformers.SharedInformerFactory, hubClientSet hubclientset.Interface, hubInformer hubinformers.SharedInformerFactory, traefikClientSet v1alpha1.TraefikV1alpha1Interface, config *WatcherGatewayConfig) *WatcherGateway {
	return &WatcherGateway{
		config: config,

		platform: client,

		kubeClientSet: kubeClientSet,
		kubeInformer:  kubeInformer,

		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,

		traefikClientSet: traefikClientSet,
	}
}

// Run runs WatcherGateway.
func (w *WatcherGateway) Run(ctx context.Context) {
	t := time.NewTicker(w.config.GatewaySyncInterval)
	defer t.Stop()

	certSyncInterval := time.After(w.config.CertSyncInterval)
	ctxSync, cancel := context.WithTimeout(ctx, 20*time.Second)
	if err := w.syncCertificates(ctxSync); err != nil {
		log.Error().Err(err).Msg("Unable to synchronize certificates with platform")
		certSyncInterval = time.After(w.config.CertRetryInterval)
	}
	w.syncGateways(ctxSync)
	cancel()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping API gateway watcher")
			return

		case <-t.C:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			w.syncGateways(ctxSync)
			cancel()

		case <-certSyncInterval:
			ctxSync, cancel = context.WithTimeout(ctx, 20*time.Second)
			if err := w.syncCertificates(ctxSync); err != nil {
				log.Error().Err(err).Msg("Unable to synchronize certificates with platform")
				certSyncInterval = time.After(w.config.CertRetryInterval)
				cancel()
				continue
			}
			certSyncInterval = time.After(w.config.CertSyncInterval)
			cancel()
		}
	}
}

func (w *WatcherGateway) syncCertificates(ctx context.Context) error {
	wildcardCert, err := w.platform.GetWildcardCertificate(ctx)
	if err != nil {
		return fmt.Errorf("get wildcardCert: %w", err)
	}

	w.wildCardCertMu.RLock()
	if bytes.Equal(wildcardCert.Certificate, w.wildCardCert.Certificate) &&
		bytes.Equal(wildcardCert.PrivateKey, w.wildCardCert.PrivateKey) {
		w.wildCardCertMu.RUnlock()

		return nil
	}
	w.wildCardCertMu.RUnlock()

	if err = w.upsertSecret(ctx, wildcardCert, hubDomainSecretName, w.config.AgentNamespace, nil); err != nil {
		return fmt.Errorf("upsert secret: %w", err)
	}

	w.wildCardCertMu.Lock()
	w.wildCardCert = wildcardCert
	w.wildCardCertMu.Unlock()

	clusterGateways, err := w.hubInformer.Hub().V1alpha1().APIGateways().Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	for _, gateway := range clusterGateways {
		apisByNamespace, err := w.apisByNamespace(ctx, gateway)
		if err != nil {
			return fmt.Errorf("unable to load gateway APIs by namespace: %w", err)
		}

		err = w.setupCertificates(ctx, gateway, apisByNamespace, wildcardCert)
		if err != nil {
			log.Error().Err(err).
				Str("name", gateway.Name).
				Str("namespace", gateway.Namespace).
				Msg("unable to setup gateway certificates")
		}
	}

	return nil
}

func (w *WatcherGateway) setupCertificates(ctx context.Context, gateway *hubv1alpha1.APIGateway, apisByNamespace map[string][]resolvedAPI, certificate edgeingress.Certificate) error {
	for namespace := range apisByNamespace {
		if err := w.upsertSecret(ctx, certificate, hubDomainSecretName, namespace, gateway); err != nil {
			return fmt.Errorf("upsert secret: %w", err)
		}
	}

	if len(gateway.Status.CustomDomains) == 0 {
		return nil
	}

	cert, err := w.platform.GetCertificateByDomains(ctx, gateway.Status.CustomDomains)
	if err != nil {
		return fmt.Errorf("get certificate by domains %q: %w", strings.Join(gateway.Status.CustomDomains, ","), err)
	}

	secretName, err := getCustomDomainSecretName(gateway.Name)
	if err != nil {
		return fmt.Errorf("get custom domains secret name: %w", err)
	}

	for namespace := range apisByNamespace {
		if err := w.upsertSecret(ctx, cert, secretName, namespace, gateway); err != nil {
			return fmt.Errorf("upsert secret: %w", err)
		}
	}

	return nil
}

func (w *WatcherGateway) upsertSecret(ctx context.Context, cert edgeingress.Certificate, name, namespace string, gateway *hubv1alpha1.APIGateway) error {
	secret, err := w.kubeClientSet.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get secret: %w", err)
	}

	if kerror.IsNotFound(err) {
		secret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "traefik-hub",
				},
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": cert.Certificate,
				"tls.key": cert.PrivateKey,
			},
		}
		if gateway != nil {
			secret.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "hub.traefik.io/v1alpha1",
				Kind:       "APIGateway",
				Name:       gateway.Name,
				UID:        gateway.UID,
			}}
		}

		_, err = w.kubeClientSet.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create secret: %w", err)
		}

		log.Debug().
			Str("name", secret.Name).
			Str("namespace", secret.Namespace).
			Msg("Secret created")

		return nil
	}

	newOwners := secret.OwnerReferences
	if gateway != nil {
		newOwners = appendOwnerReference(secret.OwnerReferences, metav1.OwnerReference{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APIGateway",
			Name:       gateway.Name,
			UID:        gateway.UID,
		})
	}
	if bytes.Equal(secret.Data["tls.crt"], cert.Certificate) && len(secret.OwnerReferences) == len(newOwners) {
		return nil
	}

	secret.Data = map[string][]byte{
		"tls.crt": cert.Certificate,
		"tls.key": cert.PrivateKey,
	}
	secret.OwnerReferences = newOwners

	_, err = w.kubeClientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	log.Debug().
		Str("name", secret.Name).
		Str("namespace", secret.Namespace).
		Msg("Secret updated")

	return nil
}

func (w *WatcherGateway) syncGateways(ctx context.Context) {
	platformGateways, err := w.platform.GetGateways(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Unable to fetch APIGateways")
		return
	}

	clusterGateways, err := w.hubInformer.Hub().V1alpha1().APIGateways().Lister().List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Msg("Unable to obtain APIGateways")
		return
	}

	gatewaysByName := map[string]*hubv1alpha1.APIGateway{}
	for _, gateway := range clusterGateways {
		gatewaysByName[gateway.Name] = gateway
	}

	for _, gateway := range platformGateways {
		platformGateway := gateway

		clusterGateway, found := gatewaysByName[platformGateway.Name]

		// Gateways that will remain in the map will be deleted.
		delete(gatewaysByName, platformGateway.Name)

		if !found {
			if err = w.createGateway(ctx, &platformGateway); err != nil {
				log.Error().Err(err).
					Str("name", platformGateway.Name).
					Msg("Unable to create APIGateway")
			}
			continue
		}

		if err = w.updateGateway(ctx, clusterGateway, &platformGateway); err != nil {
			log.Error().Err(err).
				Str("name", platformGateway.Name).
				Msg("Unable to update APIGateway")
		}
	}

	w.cleanGateways(ctx, gatewaysByName)
}

func (w *WatcherGateway) createGateway(ctx context.Context, gateway *Gateway) error {
	obj, err := gateway.Resource()
	if err != nil {
		return fmt.Errorf("build APIGateway resource: %w", err)
	}

	obj, err = w.hubClientSet.HubV1alpha1().APIGateways().Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating APIGateway: %w", err)
	}

	log.Debug().
		Str("name", obj.Name).
		Msg("APIGateway created")

	return w.syncChildResources(ctx, obj)
}

func (w *WatcherGateway) updateGateway(ctx context.Context, oldGateway *hubv1alpha1.APIGateway, newGateway *Gateway) error {
	obj, err := newGateway.Resource()
	if err != nil {
		return fmt.Errorf("build APIGateway resource: %w", err)
	}

	obj.ObjectMeta = oldGateway.ObjectMeta

	if obj.Status.Version != oldGateway.Status.Version {
		obj, err = w.hubClientSet.HubV1alpha1().APIGateways().Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating APIGateway: %w", err)
		}

		log.Debug().
			Str("name", obj.Name).
			Msg("APIGateway updated")
	}

	return w.syncChildResources(ctx, obj)
}

func (w *WatcherGateway) cleanGateways(ctx context.Context, gateways map[string]*hubv1alpha1.APIGateway) {
	for _, gateway := range gateways {
		// Foreground propagation allow us to delete all resources owned by the APIGateway.
		policy := metav1.DeletePropagationForeground

		opts := metav1.DeleteOptions{
			PropagationPolicy: &policy,
		}
		err := w.hubClientSet.HubV1alpha1().APIGateways().Delete(ctx, gateway.Name, opts)
		if err != nil {
			log.Error().Err(err).Msg("Unable to delete APIGateway")

			continue
		}

		log.Debug().
			Str("name", gateway.Name).
			Msg("APIGateway deleted")
	}
}

func (w *WatcherGateway) syncChildResources(ctx context.Context, gateway *hubv1alpha1.APIGateway) error {
	apisByNamespace, err := w.apisByNamespace(ctx, gateway)
	if err != nil {
		return fmt.Errorf("unable to load gateway APIs by namespace: %w", err)
	}

	w.wildCardCertMu.RLock()
	certificate := w.wildCardCert
	w.wildCardCertMu.RUnlock()

	if err := w.setupCertificates(ctx, gateway, apisByNamespace, certificate); err != nil {
		return fmt.Errorf("unable to setup APIGateway certificates: %w", err)
	}

	if err := w.cleanupNamespaces(ctx, gateway, apisByNamespace); err != nil {
		return fmt.Errorf("clean up ingresses: %w", err)
	}

	if err := w.upsertIngresses(ctx, gateway, apisByNamespace); err != nil {
		return fmt.Errorf("upsert ingresses: %w", err)
	}

	return nil
}

type resolvedAPI struct {
	groups string
	api    *hubv1alpha1.API
}

func (w *WatcherGateway) apisByNamespace(ctx context.Context, gateway *hubv1alpha1.APIGateway) (map[string][]resolvedAPI, error) {
	var resolvedAPIs []resolvedAPI
	for _, accessName := range gateway.Spec.APIAccesses {
		access, err := w.hubClientSet.HubV1alpha1().APIAccesses().Get(ctx, accessName, metav1.GetOptions{})
		if err != nil && kerror.IsNotFound(err) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("get access: %w", err)
		}

		apis, err := w.findAPIs(access.Spec.APISelector)
		if err != nil {
			return nil, fmt.Errorf("find APIs: %w", err)
		}

		sort.Strings(access.Spec.Groups)
		groups := strings.Join(access.Spec.Groups, ",")
		for _, api := range apis {
			resolvedAPIs = append(resolvedAPIs, resolvedAPI{groups: groups, api: api})
		}

		collections, err := w.findCollections(access.Spec.APICollectionSelector)
		if err != nil {
			return nil, fmt.Errorf("find collections: %w", err)
		}

		for _, collection := range collections {
			collectionAPIs, err := w.findAPIs(&collection.Spec.APISelector)
			if err != nil {
				return nil, fmt.Errorf("find APIs: %w", err)
			}

			for _, collectionAPI := range collectionAPIs {
				if collection.Spec.PathPrefix == "" {
					resolvedAPIs = append(resolvedAPIs, resolvedAPI{groups: groups, api: collectionAPI})
					continue
				}

				api := *collectionAPI
				api.Spec.PathPrefix = path.Join(collection.Spec.PathPrefix, api.Spec.PathPrefix)
				resolvedAPIs = append(resolvedAPIs, resolvedAPI{groups: groups, api: &api})
			}
		}
	}

	apisByNamespace := make(map[string][]resolvedAPI)
	for _, api := range resolvedAPIs {
		apisByNamespace[api.api.Namespace] = append(apisByNamespace[api.api.Namespace], api)
	}

	return apisByNamespace, nil
}

func (w *WatcherGateway) findAPIs(selector *metav1.LabelSelector) ([]*hubv1alpha1.API, error) {
	if selector == nil {
		return nil, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("convert APIs label selector: %w", err)
	}

	apis, err := w.hubInformer.Hub().V1alpha1().APIs().Lister().List(labelSelector)
	if err != nil {
		return nil, fmt.Errorf("list APIs: %w", err)
	}

	return apis, nil
}

func (w *WatcherGateway) findCollections(selector *metav1.LabelSelector) ([]*hubv1alpha1.APICollection, error) {
	if selector == nil {
		return nil, nil
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fmt.Errorf("convert collections label selector: %w", err)
	}

	collections, err := w.hubInformer.Hub().V1alpha1().APICollections().Lister().List(labelSelector)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	return collections, nil
}

func (w *WatcherGateway) upsertIngresses(ctx context.Context, gateway *hubv1alpha1.APIGateway, apisByNamespace map[string][]resolvedAPI) error {
	for namespace, resolvedAPIs := range apisByNamespace {
		traefikMiddlewareName, err := w.setupStripPrefixMiddleware(ctx, gateway.Name, resolvedAPIs, namespace)
		if err != nil {
			return fmt.Errorf("setup stripPrefix middleware: %w", err)
		}

		err = w.upsertIngressesOnNamespace(ctx, namespace, gateway, resolvedAPIs, traefikMiddlewareName)
		if err != nil {
			return fmt.Errorf("build ingress for hub domain and namespace %q: %w", namespace, err)
		}
	}

	return nil
}

func (w *WatcherGateway) setupStripPrefixMiddleware(ctx context.Context, gatewayName string, resolvedAPIS []resolvedAPI, namespace string) (string, error) {
	var apis []*hubv1alpha1.API
	for _, a := range resolvedAPIS {
		apis = append(apis, a.api)
	}

	name, err := getStripPrefixMiddlewareName(gatewayName)
	if err != nil {
		return "", fmt.Errorf("get stripPrefix middleware name: %w", err)
	}

	existingMiddleware, existingErr := w.traefikClientSet.Middlewares(namespace).Get(ctx, name, metav1.GetOptions{})
	if existingErr != nil && !kerror.IsNotFound(existingErr) {
		return "", fmt.Errorf("get middleware: %w", existingErr)
	}

	middleware := newStripPrefixMiddleware(name, namespace, apis)

	traefikMiddlewareName, err := getTraefikStripPrefixMiddlewareName(namespace, gatewayName)
	if err != nil {
		return "", fmt.Errorf("get Traefik stripPrefix middleware name: %w", err)
	}

	if kerror.IsNotFound(existingErr) {
		_, err = w.traefikClientSet.Middlewares(namespace).Create(ctx, &middleware, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("create middleware: %w", err)
		}

		log.Debug().
			Str("name", name).
			Str("namespace", namespace).
			Msg("Middleware created")

		return traefikMiddlewareName, nil
	}

	if middleware.Spec == existingMiddleware.Spec {
		return traefikMiddlewareName, nil
	}

	existingMiddleware.Spec = middleware.Spec

	_, err = w.traefikClientSet.Middlewares(existingMiddleware.Namespace).Update(ctx, existingMiddleware, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("update middleware: %w", err)
	}

	return traefikMiddlewareName, nil
}

func (w *WatcherGateway) upsertIngress(ctx context.Context, ingress *netv1.Ingress) error {
	existingIngress, err := w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Get(ctx, ingress.Name, metav1.GetOptions{})
	if err != nil && !kerror.IsNotFound(err) {
		return fmt.Errorf("get ingress: %w", err)
	}

	if kerror.IsNotFound(err) {
		_, err = w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ingress: %w", err)
		}

		log.Debug().
			Str("name", ingress.Name).
			Str("namespace", ingress.Namespace).
			Msg("Ingress created")

		return nil
	}

	existingIngress.Spec = ingress.Spec
	// Override Annotations and Labels in case new values are added in the future.
	existingIngress.ObjectMeta.Annotations = ingress.ObjectMeta.Annotations
	existingIngress.ObjectMeta.Labels = ingress.ObjectMeta.Labels

	_, err = w.kubeClientSet.NetworkingV1().Ingresses(ingress.Namespace).Update(ctx, existingIngress, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update ingress: %w", err)
	}

	log.Debug().
		Str("name", ingress.Name).
		Str("namespace", ingress.Namespace).
		Msg("Ingress updated")

	return nil
}

// cleanupNamespaces cleans namespace from APIGateway resources that are no longer referenced.
func (w *WatcherGateway) cleanupNamespaces(ctx context.Context, gateway *hubv1alpha1.APIGateway, apisByNamespace map[string][]resolvedAPI) error {
	managedByHub, err := labels.NewRequirement("app.kubernetes.io/managed-by", selection.Equals, []string{"traefik-hub"})
	if err != nil {
		return fmt.Errorf("create managed by hub requirement: %w", err)
	}
	hubIngressesSelector := labels.NewSelector().Add(*managedByHub)
	hubIngresses, err := w.kubeInformer.Networking().V1().Ingresses().Lister().List(hubIngressesSelector)
	if err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}

	ingressName, err := getIngressName(gateway.Name)
	if err != nil {
		return fmt.Errorf("get ingress name for hub domain: %w", err)
	}

	for _, ingress := range hubIngresses {
		if !strings.HasPrefix(ingress.Name, ingressName) {
			continue
		}

		logger := log.Ctx(ctx).With().Str("gateway_name", gateway.Name).Str("namespace", ingress.Namespace).Logger()
		if _, ok := apisByNamespace[ingress.Namespace]; !ok {
			err = w.kubeClientSet.CoreV1().
				Secrets(ingress.Namespace).
				Delete(ctx, ingress.Spec.TLS[0].SecretName, metav1.DeleteOptions{})
			if err != nil && !kerror.IsNotFound(err) {
				logger.Error().Err(err).
					Str("secret_name", ingress.Spec.TLS[0].SecretName).
					Msg("Unable to clean APIGateway's child Secret")

				continue
			}

			middlewareName, err := getStripPrefixMiddlewareName(gateway.Name)
			if err != nil {
				logger.Error().Err(err).Msg("Unable to get APIGateway's child Middleware name")

				continue
			}

			err = w.traefikClientSet.
				Middlewares(ingress.Namespace).
				Delete(ctx, middlewareName, metav1.DeleteOptions{})
			if err != nil && !kerror.IsNotFound(err) {
				logger.Error().Err(err).
					Str("middleware_name", middlewareName).
					Msg("Unable to clean APIGateway's child Middleware")

				continue
			}

			err = w.kubeClientSet.NetworkingV1().
				Ingresses(ingress.Namespace).
				Delete(ctx, ingress.Name, metav1.DeleteOptions{})
			if err != nil && !kerror.IsNotFound(err) {
				logger.Error().Err(err).
					Str("ingress_name", ingress.Name).
					Msg("Unable to clean APIGateway's child Ingress")
			}
		}
	}

	return nil
}

func (w *WatcherGateway) upsertIngressesOnNamespace(ctx context.Context, namespace string, gateway *hubv1alpha1.APIGateway, resolvedAPIs []resolvedAPI, traefikMiddlewareName string) error {
	managedByHub, err := labels.NewRequirement("app.kubernetes.io/managed-by", selection.Equals, []string{"traefik-hub"})
	if err != nil {
		return fmt.Errorf("create managed by hub requirement: %w", err)
	}
	hubIngressesSelector := labels.NewSelector().Add(*managedByHub)

	list, err := w.kubeInformer.Networking().V1().Ingresses().Lister().Ingresses(namespace).List(hubIngressesSelector)
	if err != nil {
		return fmt.Errorf("unable to list ingresses: %w", err)
	}

	apisByGroups := make(map[string][]*hubv1alpha1.API)
	for _, a := range resolvedAPIs {
		apisByGroups[a.groups] = append(apisByGroups[a.groups], a.api)
	}

	ingressUpserted := make(map[string]struct{})
	for groups, apis := range apisByGroups {
		name, err := getHubDomainIngressName(gateway.Name, groups)
		if err != nil {
			return fmt.Errorf("get hub domain ingress name: %w", err)
		}

		var paths []netv1.HTTPIngressPath
		pathType := netv1.PathTypePrefix
		for _, api := range apis {
			paths = append(paths, netv1.HTTPIngressPath{
				PathType: &pathType,
				Path:     api.Spec.PathPrefix,
				Backend: netv1.IngressBackend{
					Service: &netv1.IngressServiceBackend{
						Name: api.Spec.Service.Name,
						Port: netv1.ServiceBackendPort(api.Spec.Service.Port),
					},
				},
			})
		}

		rules := []netv1.IngressRule{{
			Host: gateway.Status.HubDomain,
			IngressRuleValue: netv1.IngressRuleValue{
				HTTP: &netv1.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		}}

		ing := &netv1.Ingress{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "networking.k8s.io/v1",
				Kind:       "Ingress",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Annotations: map[string]string{
					"traefik.ingress.kubernetes.io/router.tls":         "true",
					"traefik.ingress.kubernetes.io/router.entrypoints": w.config.TraefikTunnelEntryPoint,
					"traefik.ingress.kubernetes.io/router.middlewares": traefikMiddlewareName,
					reviewer.AnnotationHubAuth:                         "hub-api-management",
					reviewer.AnnotationHubAuthGroup:                    groups,
				},
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "traefik-hub",
				},
				// Set OwnerReference allow us to delete ingresses owned by an APIGateway.
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: gateway.APIVersion,
					Kind:       gateway.Kind,
					Name:       gateway.Name,
					UID:        gateway.UID,
				}},
			},
			Spec: netv1.IngressSpec{
				IngressClassName: pointer.String(w.config.IngressClassName),
				Rules:            rules,
				TLS: []netv1.IngressTLS{{
					Hosts:      []string{gateway.Status.HubDomain},
					SecretName: hubDomainSecretName,
				}},
			},
		}

		if err = w.upsertIngress(ctx, ing); err != nil {
			return fmt.Errorf("upsert ingress for hub domain and namespace %q: %w", namespace, err)
		}
		ingressUpserted[name] = struct{}{}

		if len(gateway.Status.CustomDomains) == 0 {
			continue
		}

		name, err = getCustomDomainsIngressName(gateway.Name, groups)
		if err != nil {
			return fmt.Errorf("get custom domains ingress name: %w", err)
		}

		ing.Name = name
		ing.ObjectMeta.Annotations["traefik.ingress.kubernetes.io/router.entrypoints"] = w.config.TraefikAPIEntryPoint

		var rulesCustom []netv1.IngressRule
		for _, domain := range gateway.Status.CustomDomains {
			rulesCustom = append(rulesCustom, netv1.IngressRule{
				Host: domain,
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: paths,
					},
				},
			})
		}

		ing.Spec.Rules = rulesCustom

		secretName, err := getCustomDomainSecretName(gateway.Name)
		if err != nil {
			return fmt.Errorf("get custom domains secret name: %w", err)
		}

		ing.Spec.TLS = []netv1.IngressTLS{{
			Hosts:      gateway.Status.CustomDomains,
			SecretName: secretName,
		}}

		if err = w.upsertIngress(ctx, ing); err != nil {
			return fmt.Errorf("upsert ingress for custom domain and namespace %q: %w", namespace, err)
		}
		ingressUpserted[name] = struct{}{}
	}

	log.Debug().
		Str("namespace", namespace).
		Interface("ingresses_upserted", ingressUpserted).
		Int("upserted_ingresses_count", len(ingressUpserted)).
		Msg("upserted ingresses")

	for _, oldIngress := range list {
		if _, found := ingressUpserted[oldIngress.Name]; !found {
			if err := w.kubeClientSet.NetworkingV1().Ingresses(namespace).Delete(ctx, oldIngress.Name, metav1.DeleteOptions{}); err != nil {
				log.Error().Err(err).
					Str("namespace", namespace).
					Str("ingress_name", oldIngress.Name).
					Msg("Unable to delete ingress")
			}
		}
	}

	return nil
}

func newStripPrefixMiddleware(name, namespace string, apis []*hubv1alpha1.API) traefikv1alpha1.Middleware {
	var prefixes []string
	for _, api := range apis {
		prefixes = append(prefixes, api.Spec.PathPrefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})

	return traefikv1alpha1.Middleware{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Middleware",
			APIVersion: "traefik.containo.us/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: traefikv1alpha1.MiddlewareSpec{
			StripPrefix: &traefikv1alpha1.StripPrefix{
				Prefixes:   prefixes,
				ForceSlash: false,
			},
		},
	}
}

// getStripPrefixMiddlewareName compute the name of the stripPrefix middleware.
// The name follow this format: {{gateway-name}-hash({gateway-name})-stripprefix}
// This hash is here to reduce the chance of getting a collision on an existing secret while staying under
// the limit of 63 characters.
func getStripPrefixMiddlewareName(gatewayName string) (string, error) {
	h, err := hash(gatewayName)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-stripprefix", gatewayName, h), nil
}

func getTraefikStripPrefixMiddlewareName(namespace, gatewayName string) (string, error) {
	middlewareName, err := getStripPrefixMiddlewareName(gatewayName)
	if err != nil {
		return "", fmt.Errorf("get stripPrefix middleware name: %w", err)
	}
	return fmt.Sprintf("%s-%s@kubernetescrd", namespace, middlewareName), nil
}

// getHubDomainIngressName compute the ingress name for hub domain from the gateway name.
// The name follow this format: {gateway-name}-{hash(gateway-name)}-{hash(groups)}-hub
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getHubDomainIngressName(name, groups string) (string, error) {
	h, err := hash(groups)
	if err != nil {
		return "", err
	}

	name, err = getIngressName(name)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d-hub", name, h), nil
}

// getCustomDomainsIngressName compute the ingress name for custom domains from the gateway name.
// The name follow this format: {gateway-name}-{hash(gateway-name)}-{hash(groups)}
// This hash is here to reduce the chance of getting a collision on an existing ingress.
func getCustomDomainsIngressName(name, groups string) (string, error) {
	h, err := hash(groups)
	if err != nil {
		return "", err
	}

	name, err = getIngressName(name)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", name, h), nil
}

func getIngressName(name string) (string, error) {
	h, err := hash(name)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", name, h), nil
}

// getCustomDomainSecretName compute the name of the secret storing the certificate of the custom domains.
// The name follow this format: {customDomainSecretNamePrefix}-{hash(gateway-name)}
// This hash is here to reduce the chance of getting a collision on an existing secret while staying under
// the limit of 63 characters.
func getCustomDomainSecretName(name string) (string, error) {
	h, err := hash(name)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%d", customDomainSecretNamePrefix, h), nil
}

func appendOwnerReference(references []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	for _, reference := range references {
		if reference.String() == ref.String() {
			return references
		}
	}

	return append(references, ref)
}
