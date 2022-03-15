package acme

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	traefikinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/informers/externalversions"
	"github.com/traefik/hub-agent-kubernetes/pkg/kubevers"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"
)

// controllerName is the name of the controller.
const controllerName = "hub"

// annotationHubEnableACME is an opt-in annotation which has to be added to an Ingress/IngressRoute to indicate that its TLS secrets have to be managed by the agent.
const annotationHubEnableACME = "hub.traefik.io/enable-acme"

// annotationIngressClass is an annotation used to specify the Ingress class before Kubernetes 1.18.
const annotationIngressClass = "kubernetes.io/ingress.class"

// Supported default IngressClass annotation values.
const (
	defaultAnnotationHAProxy = "haproxy"
	defaultAnnotationNginx   = "nginx"
	defaultAnnotationTraefik = "traefik"
)

// annotationDefaultIngressClass is the annotation added on an ingress class to indicate that it is the default one.
const annotationDefaultIngressClass = "ingressclass.kubernetes.io/is-default-class"

// Supported Ingress controller types.
const (
	controllerTypeHAProxyCommunity = "haproxy-ingress.github.io/controller"
	controllerTypeNginxOfficial    = "nginx.org/ingress-controller"
	controllerTypeNginxCommunity   = "k8s.io/ingress-nginx"
	controllerTypeTraefik          = "traefik.io/ingress-controller"
)

// defaultResync is the resync events interval.
const defaultResync = 5 * time.Minute

// CertIssuer is responsible of obtaining and storing certificate in secrets.
type CertIssuer interface {
	ObtainCertificate(req CertificateRequest)
}

// Controller is a Kubernetes controller listening to events in order to detect the certificates needed by an Ingress.
type Controller struct {
	certs            CertIssuer
	cacheSyncChan    chan struct{}
	kubeClient       clientset.Interface
	kubeInformers    informers.SharedInformerFactory
	hubInformers     hubinformer.SharedInformerFactory
	traefikInformers traefikinformer.SharedInformerFactory
}

// NewController returns a new Controller instance.
func NewController(certs CertIssuer, kubeClient clientset.Interface, hubClient hubclientset.Interface, traefikClient traefikclientset.Interface) (*Controller, error) {
	serverVersionInfo, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("get server version: %w", err)
	}

	serverVersion := serverVersionInfo.GitVersion
	kubeInformers := informers.NewSharedInformerFactory(kubeClient, defaultResync)
	hubInformers := hubinformer.NewSharedInformerFactory(hubClient, defaultResync)
	traefikInformers := traefikinformer.NewSharedInformerFactory(traefikClient, defaultResync)

	ctrl := &Controller{
		certs:            certs,
		cacheSyncChan:    make(chan struct{}),
		kubeClient:       kubeClient,
		kubeInformers:    kubeInformers,
		hubInformers:     hubInformers,
		traefikInformers: traefikInformers,
	}

	kubeInformers.Core().V1().Secrets().Informer().AddEventHandler(DelayedResourceEventHandler{
		SyncChan: ctrl.cacheSyncChan,
		Handler: cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				secret, ok := obj.(*corev1.Secret)
				return ok && isManagedSecret(secret)
			},
			Handler: cache.ResourceEventHandlerFuncs{
				DeleteFunc: ctrl.secretDeleted,
			},
		},
	})

	ingressEventHandler := DelayedResourceEventHandler{
		SyncChan: ctrl.cacheSyncChan,
		Handler: cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				ing, asErr := asIngressV1(obj)
				if asErr != nil {
					log.Error().Err(asErr).Msg("Unable to convert object to Ingress")
					return false
				}
				return isACMEEnabled(&ing.ObjectMeta) && ctrl.isSupportedIngressController(ing)
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    ctrl.ingressCreated,
				UpdateFunc: ctrl.ingressUpdated,
				DeleteFunc: ctrl.ingressDeleted,
			},
		},
	}

	if kubevers.SupportsNetV1IngressClasses(serverVersion) {
		kubeInformers.Networking().V1().IngressClasses().Informer()
	} else if kubevers.SupportsNetV1Beta1IngressClasses(serverVersion) {
		kubeInformers.Networking().V1beta1().IngressClasses().Informer()
	}

	if kubevers.SupportsNetV1Ingresses(serverVersion) {
		kubeInformers.Networking().V1().Ingresses().Informer().AddEventHandler(ingressEventHandler)
	} else {
		// Since we only support Kubernetes v1.14 and up, we always have at least net v1beta1 Ingresses.
		kubeInformers.Networking().V1beta1().Ingresses().Informer().AddEventHandler(ingressEventHandler)
	}

	hubInformers.Hub().V1alpha1().IngressClasses().Informer()

	hasIngressRoute, err := hasTraefikCRDs(kubeClient)
	if err != nil {
		return nil, fmt.Errorf("check presence of Traefik IngressRoute CRD: %w", err)
	}
	if !hasIngressRoute {
		return ctrl, nil
	}

	traefikInformers.Traefik().V1alpha1().IngressRoutes().Informer().AddEventHandler(DelayedResourceEventHandler{
		SyncChan: ctrl.cacheSyncChan,
		Handler: cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				ingRoute, ok := (obj).(*traefikv1alpha1.IngressRoute)
				return ok && isACMEEnabled(&ingRoute.ObjectMeta)
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    ctrl.ingressRouteCreated,
				UpdateFunc: ctrl.ingressRouteUpdated,
				DeleteFunc: ctrl.ingressRouteDeleted,
			},
		},
	})

	return ctrl, nil
}

// Run starts the controller routine.
func (c *Controller) Run(ctx context.Context) error {
	c.kubeInformers.Start(ctx.Done())
	c.hubInformers.Start(ctx.Done())
	c.traefikInformers.Start(ctx.Done())

	for typ, ok := range c.kubeInformers.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for k8s object caches to sync %s", typ)
		}
	}

	for typ, ok := range c.hubInformers.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for Hub object caches to sync %s", typ)
		}
	}

	for typ, ok := range c.traefikInformers.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("timed out waiting for Traefik object caches to sync %s", typ)
		}
	}

	// Here we close the cacheSyncChan to indicate that all caches are sync and that handlers can start processing events.
	close(c.cacheSyncChan)

	<-ctx.Done()

	return nil
}

func (c *Controller) secretDeleted(obj interface{}) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Error().Msgf("Couldn't get object from tombstone %#v", obj)
			return
		}
		secret, ok = tombstone.Obj.(*corev1.Secret)
		if !ok {
			log.Error().Msgf("Tombstone contained object that is not a secret %#v", obj)
			return
		}
	}

	used, err := c.isSecretUsed(secret)
	if err != nil {
		log.Error().
			Err(err).
			Str("namespace", secret.Namespace).
			Str("secret", secret.Name).
			Msg("Unable to check secret usage")

		return
	}
	if !used {
		return
	}

	c.certs.ObtainCertificate(CertificateRequest{
		Domains:    getCertificateDomains(secret),
		Namespace:  secret.Namespace,
		SecretName: secret.Name,
	})
}

func (c *Controller) deleteUnusedSecrets(namespace string, names ...string) {
	for _, name := range names {
		logger := log.With().
			Str("namespace", namespace).
			Str("secret", name).
			Logger()

		secret, err := c.kubeInformers.Core().V1().Secrets().Lister().Secrets(namespace).Get(name)
		if err != nil && !kerror.IsNotFound(err) {
			logger.Error().Err(err).Msg("Unable to get secret")
			continue
		}
		if secret == nil || !isManagedSecret(secret) {
			continue
		}

		used, err := c.isSecretUsed(secret)
		if err != nil {
			logger.Error().Err(err).Msg("Unable to check secret usage")
			return
		}
		if used {
			continue
		}

		err = c.kubeClient.CoreV1().Secrets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil && !kerror.IsNotFound(err) {
			logger.Error().Err(err).Msg("Unable to delete secret")
		}
	}
}

func (c *Controller) isSupportedIngressController(ing *netv1.Ingress) bool {
	if hasDefaultIngressClassAnnotation(ing) {
		return true
	}

	if ing.Spec.IngressClassName == nil || pointer.StringPtrDerefOr(ing.Spec.IngressClassName, "") == "" {
		ctrl, err := c.getDefaultIngressClassController()
		if err != nil {
			log.Error().Err(err).Msg("Unable to get default IngressClass")
			return false
		}

		return isSupportedController(ctrl)
	}

	ctrl, err := c.getIngressClassController(pointer.StringPtrDerefOr(ing.Spec.IngressClassName, ""))
	if err != nil {
		log.Error().Err(err).Msg("Unable to get IngressClass")
		return false
	}

	return isSupportedController(ctrl)
}

func (c *Controller) getDefaultIngressClassController() (string, error) {
	hubIngClasses, err := c.hubInformers.Hub().V1alpha1().IngressClasses().Lister().List(labels.Everything())
	if err != nil {
		return "", fmt.Errorf("list hub IngressClasses: %w", err)
	}

	for _, ingClass := range hubIngClasses {
		if ingClass.Annotations[annotationDefaultIngressClass] == "true" {
			return ingClass.Spec.Controller, nil
		}
	}

	ingClasses, err := c.kubeInformers.Networking().V1().IngressClasses().Lister().List(labels.Everything())
	if err != nil {
		return "", fmt.Errorf("list v1 IngressClasses: %w", err)
	}

	for _, ingClass := range ingClasses {
		if ingClass.Annotations[annotationDefaultIngressClass] == "true" {
			return ingClass.Spec.Controller, nil
		}
	}

	bIngClasses, err := c.kubeInformers.Networking().V1beta1().IngressClasses().Lister().List(labels.Everything())
	if err != nil {
		return "", fmt.Errorf("list v1beta1 IngressClasses: %w", err)
	}

	for _, bIngClass := range bIngClasses {
		if bIngClass.Annotations[annotationDefaultIngressClass] == "true" {
			return bIngClass.Spec.Controller, nil
		}
	}

	return "", nil
}

func (c *Controller) getIngressClassController(name string) (string, error) {
	hubIng, err := c.hubInformers.Hub().V1alpha1().IngressClasses().Lister().Get(name)
	if err != nil && !kerror.IsNotFound(err) {
		return "", fmt.Errorf("get hub IngressClass: %w", err)
	}
	if hubIng != nil {
		return hubIng.Spec.Controller, nil
	}

	ing, err := c.kubeInformers.Networking().V1().IngressClasses().Lister().Get(name)
	if err != nil && !kerror.IsNotFound(err) {
		return "", fmt.Errorf("get v1 IngressClass: %w", err)
	}
	if ing != nil {
		return ing.Spec.Controller, nil
	}

	bIng, err := c.kubeInformers.Networking().V1beta1().IngressClasses().Lister().Get(name)
	if err != nil && !kerror.IsNotFound(err) {
		return "", fmt.Errorf("get v1beta1 IngressClass: %w", err)
	}
	if bIng != nil {
		return bIng.Spec.Controller, nil
	}

	return "", nil
}

func (c *Controller) isSecretUsed(secret *corev1.Secret) (bool, error) {
	ings, err := c.listIngresses(secret.Namespace)
	if err != nil {
		return false, err
	}

	for _, ing := range ings {
		if !isACMEEnabled(&ing.ObjectMeta) {
			continue
		}

		for _, tls := range ing.Spec.TLS {
			if tls.SecretName == secret.Name {
				return true, nil
			}
		}
	}

	ingRoutes, err := c.traefikInformers.Traefik().V1alpha1().IngressRoutes().Lister().IngressRoutes(secret.Namespace).List(labels.Everything())
	if err != nil {
		return false, fmt.Errorf("list IngressRoutes: %w", err)
	}

	for _, ingRoute := range ingRoutes {
		if !isACMEEnabled(&ingRoute.ObjectMeta) {
			continue
		}

		if ingRoute.Spec.TLS != nil && ingRoute.Spec.TLS.SecretName == secret.Name {
			return true, nil
		}
	}

	return false, nil
}

// listIngresses returns a unified list of the v1beta1 and v1 Ingresses in the given namespace.
func (c *Controller) listIngresses(namespace string) ([]*netv1.Ingress, error) {
	var result []*netv1.Ingress

	bIngs, err := c.kubeInformers.Networking().V1beta1().Ingresses().Lister().Ingresses(namespace).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list v1beta1 Ingresses: %w", err)
	}

	for _, bIng := range bIngs {
		ing, marshalErr := marshalToIngressNetworkingV1(bIng)
		if marshalErr != nil {
			return nil, marshalErr
		}
		result = append(result, ing)
	}

	ings, err := c.kubeInformers.Networking().V1().Ingresses().Lister().Ingresses(namespace).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list v1 Ingresses: %w", err)
	}

	return append(result, ings...), nil
}

// marshalToIngressNetworkingV1 marshals only the common properties which contains the TLS configuration of an Ingress v1beta1 into an Ingress v1.
func marshalToIngressNetworkingV1(bIng *netv1beta1.Ingress) (*netv1.Ingress, error) {
	data, err := bIng.Marshal()
	if err != nil {
		return nil, fmt.Errorf("mashal v1beta1 Ingress: %w", err)
	}

	ing := &netv1.Ingress{}
	if err := ing.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("unmarshal v1 Ingress: %w", err)
	}

	return ing, nil
}

func hasTraefikCRDs(kubeClient clientset.Interface) (bool, error) {
	resourceList, err := kubeClient.Discovery().ServerResourcesForGroupVersion(traefikv1alpha1.SchemeGroupVersion.String())
	if err != nil {
		if kerror.IsNotFound(err) ||
			// because the fake client doesn't return the right error type.
			strings.HasSuffix(err.Error(), " not found") {
			return false, nil
		}
		return false, fmt.Errorf("get server groups and resources: %w", err)
	}

	for _, r := range resourceList.APIResources {
		if r.Kind == "IngressRoute" {
			return true, nil
		}
	}
	return false, nil
}

func isACMEEnabled(meta *metav1.ObjectMeta) bool {
	return meta.Annotations[annotationHubEnableACME] == "true"
}

func isSupportedController(ctrl string) bool {
	switch ctrl {
	case controllerTypeHAProxyCommunity:
	case controllerTypeNginxOfficial:
	case controllerTypeNginxCommunity:
	case controllerTypeTraefik:
	default:
		return false
	}

	return true
}

func hasDefaultIngressClassAnnotation(ing *netv1.Ingress) bool {
	switch ing.Annotations[annotationIngressClass] {
	case defaultAnnotationHAProxy:
	case defaultAnnotationNginx:
	case defaultAnnotationTraefik:
	default:
		return false
	}
	return true
}

// sanitizeDomains returns a sorted, lower cased and deduplicated domain list.
func sanitizeDomains(domains []string) []string {
	var result []string
	existingDomains := map[string]struct{}{}

	for _, domain := range domains {
		domain = strings.ToLower(domain)
		if _, exists := existingDomains[domain]; exists {
			continue
		}

		result = append(result, domain)
		existingDomains[domain] = struct{}{}
	}

	sort.Strings(result)
	return result
}
