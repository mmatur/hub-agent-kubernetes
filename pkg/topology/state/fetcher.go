package state

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/rs/zerolog/log"
	traefikv1alpha1 "github.com/traefik/hub-agent/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/informers/externalversions"
	traefikclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/clientset/versioned"
	traefikinformer "github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/informers/externalversions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/external-dns/source"
)

// Fetcher fetches Kubernetes resources and converts them into a filtered and simplified state.
type Fetcher struct {
	clusterID     string
	serverVersion *version.Version

	k8s           informers.SharedInformerFactory
	hub           hubinformer.SharedInformerFactory
	traefik       traefikinformer.SharedInformerFactory
	ingressSource source.Source
	crdSource     source.Source
	clientSet     clientset.Interface
}

// NewFetcher creates a new Fetcher.
func NewFetcher(ctx context.Context, clusterID string) (*Fetcher, error) {
	var (
		config *rest.Config
		err    error
	)

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		log.Debug().Msg("Creating in-cluster k8s client")

		config, err = rest.InClusterConfig()
	} else {
		log.Debug().Str("kubeconfig", os.Getenv("KUBECONFIG")).Msg("Creating external k8s client")

		config, err = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	}
	if err != nil {
		return nil, fmt.Errorf("create k8s configuration: %w", err)
	}

	clientSet, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// TODO: Handle ingressClasses from Hub as well.
	hubClientSet, err := hubclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	traefikClientSet, err := traefikclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	serverVersion, err := clientSet.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("get server version: %w", err)
	}

	return watchAll(ctx, clientSet, hubClientSet, traefikClientSet, serverVersion.GitVersion, clusterID)
}

func watchAll(ctx context.Context, clientSet clientset.Interface, hubClientSet hubclientset.Interface, traefikClientSet traefikclientset.Interface, serverVersion, clusterID string) (*Fetcher, error) {
	serverSemVer, err := version.NewVersion(serverVersion)
	if err != nil {
		return nil, fmt.Errorf("parse server version: %w", err)
	}

	if serverSemVer.LessThan(version.Must(version.NewVersion("1.14"))) {
		return nil, fmt.Errorf("unsupported version: %s", serverSemVer)
	}

	kubernetesFactory := informers.NewSharedInformerFactoryWithOptions(clientSet, 5*time.Minute)

	kubernetesFactory.Apps().V1().DaemonSets().Informer()
	kubernetesFactory.Apps().V1().Deployments().Informer()
	kubernetesFactory.Apps().V1().ReplicaSets().Informer()
	kubernetesFactory.Apps().V1().StatefulSets().Informer()
	kubernetesFactory.Core().V1().Endpoints().Informer()
	kubernetesFactory.Core().V1().Namespaces().Informer()
	kubernetesFactory.Core().V1().Pods().Informer()
	kubernetesFactory.Core().V1().Services().Informer()

	if serverSemVer.GreaterThanOrEqual(version.Must(version.NewVersion("1.19"))) {
		// Clusters supporting networking.k8s.io/v1.
		kubernetesFactory.Networking().V1().IngressClasses().Informer()
		kubernetesFactory.Networking().V1().Ingresses().Informer()
	}

	if serverSemVer.GreaterThanOrEqual(version.Must(version.NewVersion("1.18"))) {
		kubernetesFactory.Networking().V1beta1().IngressClasses().Informer()
	}

	if serverSemVer.LessThanOrEqual(version.Must(version.NewVersion("1.18"))) {
		kubernetesFactory.Networking().V1beta1().Ingresses().Informer()
	}

	traefikFactory := traefikinformer.NewSharedInformerFactoryWithOptions(traefikClientSet, 5*time.Minute)

	hasTraefikCRDs, err := hasTraefikCRDs(clientSet.Discovery())
	if err != nil {
		log.Error().Err(err).Msg("Unable to check if Traefik CRDs are installed")
	}
	if hasTraefikCRDs {
		traefikFactory.Traefik().V1alpha1().IngressRoutes().Informer()
		traefikFactory.Traefik().V1alpha1().TraefikServices().Informer()
	}

	ingressSource, err := source.NewIngressSource(clientSet, "", "", "", false, false, false)
	if err != nil {
		log.Error().Err(err).Msg("Unable to create external DNS IngressSource")
	}

	restClient, scheme, err := source.NewCRDClientForAPIVersionKind(clientSet, "", "", "externaldns.k8s.io/v1alpha1", "DNSEndpoint")
	if err != nil && !strings.Contains(err.Error(), "the server could not find the requested resource") {
		// The second condition above is due to the extensions package not wrapping the error correctly.
		log.Error().Err(err).Msg("Unable to fetch DNS endpoints from Kubernetes REST API")
	}

	crdSource := source.NewEmptySource()
	if restClient != nil {
		crdSource, err = source.NewCRDSource(restClient, "", "DNSEndpoint", "", "", scheme)
		if err != nil {
			log.Error().Err(err).Msg("Unable to create external DNS CRDSource")
		}
	}

	hubFactory := hubinformer.NewSharedInformerFactoryWithOptions(hubClientSet, 5*time.Minute)
	hubFactory.Hub().V1alpha1().AccessControlPolicies().Informer()

	kubernetesFactory.Start(ctx.Done())
	hubFactory.Start(ctx.Done())
	traefikFactory.Start(ctx.Done())

	for typ, ok := range kubernetesFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for k8s object caches to sync %s", typ)
		}
	}

	for typ, ok := range hubFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for access control policies caches to sync %s", typ)
		}
	}

	for typ, ok := range traefikFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for Traefik CRD caches to sync %s", typ)
		}
	}

	return &Fetcher{
		clusterID:     clusterID,
		serverVersion: serverSemVer,
		k8s:           kubernetesFactory,
		hub:           hubFactory,
		traefik:       traefikFactory,
		ingressSource: ingressSource,
		crdSource:     crdSource,
		clientSet:     clientSet,
	}, nil
}

// FetchState assembles a cluster state from Kubernetes resources.
func (f *Fetcher) FetchState(ctx context.Context) (*Cluster, error) {
	cluster := &Cluster{
		ID:            f.clusterID,
		ExternalDNSes: make(map[string]*ExternalDNS),
	}

	var err error

	cluster.Namespaces, err = f.getNamespaces()
	if err != nil {
		return nil, err
	}

	cluster.Apps, err = f.getApps()
	if err != nil {
		return nil, err
	}

	cluster.Services, cluster.TraefikServiceNames, err = f.getServices(cluster.Apps)
	if err != nil {
		return nil, err
	}

	// getIngressControllers should be called after getServices because it depends on service information.
	cluster.IngressControllers, err = f.getIngressControllers(cluster.Services, cluster.Apps)
	if err != nil {
		return nil, err
	}

	cluster.Ingresses, err = f.getIngresses(cluster.ID)
	if err != nil {
		return nil, err
	}

	cluster.IngressRoutes, err = f.getIngressRoutes(cluster.ID)
	if err != nil {
		return nil, err
	}

	ingressExternalDNSes, err := getExternalDNS(ctx, f.ingressSource)
	if err != nil {
		return nil, err
	}

	for dnsName, externalDNS := range ingressExternalDNSes {
		cluster.ExternalDNSes[objectKey(dnsName, "ingress")] = externalDNS
	}

	crdExternalDNSes, err := getExternalDNS(ctx, f.crdSource)
	if err != nil {
		return nil, err
	}

	for dnsName, externalDNS := range crdExternalDNSes {
		cluster.ExternalDNSes[objectKey(dnsName, "crd")] = externalDNS
	}

	cluster.AccessControlPolicies, err = f.getAccessControlPolicies(cluster.ID)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func hasTraefikCRDs(clientSet discovery.DiscoveryInterface) (bool, error) {
	crdList, err := clientSet.ServerResourcesForGroupVersion(traefikv1alpha1.SchemeGroupVersion.String())
	if err != nil {
		return false, err
	}

	for _, kind := range []string{ResourceKindIngressRoute, ResourceKindTraefikService} {
		var exists bool
		for _, resource := range crdList.APIResources {
			if resource.Kind == kind {
				exists = true
				break
			}
		}

		if !exists {
			return false, nil
		}
	}

	return true, nil
}

func objectKey(name, ns string) string {
	return name + "@" + ns
}

func ingressKey(meta ResourceMeta) string {
	return fmt.Sprintf("%s.%s.%s", objectKey(meta.Name, meta.Namespace), strings.ToLower(meta.Kind), meta.Group)
}

func sanitizeAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}

	result := make(map[string]string)
	for name, value := range annotations {
		if name == "kubectl.kubernetes.io/last-applied-configuration" {
			continue
		}

		result[name] = value
	}

	return result
}
