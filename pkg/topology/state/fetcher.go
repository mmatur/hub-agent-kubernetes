package state

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/rs/zerolog/log"
	acpclient "github.com/traefik/neo-agent/pkg/crd/generated/client/clientset/versioned"
	acp "github.com/traefik/neo-agent/pkg/crd/generated/client/informers/externalversions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/external-dns/source"
)

// Fetcher fetches Kubernetes resources and converts them into a filtered and simplified state.
type Fetcher struct {
	serverVersion *version.Version

	k8s           informers.SharedInformerFactory
	acp           acp.SharedInformerFactory
	ingressSource source.Source
	crdSource     source.Source
}

// NewFetcher creates a new Fetcher.
func NewFetcher(ctx context.Context) (*Fetcher, error) {
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

	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// TODO: Handle ingressClasses from Neo as well.
	acpClientSet, err := acpclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	serverVersion, err := clientSet.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("get server version: %w", err)
	}

	return watchAll(ctx, clientSet, acpClientSet, serverVersion.GitVersion)
}

func watchAll(ctx context.Context, clientSet kubernetes.Interface, acpClientSet acpclient.Interface, serverVersion string) (*Fetcher, error) {
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

	ingressSource, err := source.NewIngressSource(clientSet, "", "", "", false, false, false)
	if err != nil {
		log.Error().Err(err).Msg("fetch DNS endpoints from Kubernetes API")
	}

	restClient, scheme, err := source.NewCRDClientForAPIVersionKind(clientSet, "", "", "externaldns.k8s.io/v1alpha1", "DNSEndpoint")
	if err != nil && !strings.Contains(err.Error(), "the server could not find the requested resource") {
		// The second condition above is due to the extensions package not wrapping the error correctly.
		log.Error().Err(err).Msg("fetch DNS endpoints from Kubernetes REST API")
	}

	crdSource := source.NewEmptySource()
	if restClient != nil {
		crdSource, err = source.NewCRDSource(restClient, "", "DNSEndpoint", "", "", scheme)
		if err != nil {
			log.Error().Err(err).Msg("fetch DNS endpoints from Kubernetes CRDs")
		}
	}

	acpFactory := acp.NewSharedInformerFactoryWithOptions(acpClientSet, 5*time.Minute)
	acpFactory.Neo().V1alpha1().AccessControlPolicies().Informer()

	kubernetesFactory.Start(ctx.Done())
	acpFactory.Start(ctx.Done())

	for typ, ok := range kubernetesFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for k8s object caches to sync %s", typ)
		}
	}

	for typ, ok := range acpFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return nil, fmt.Errorf("timed out waiting for access control policies caches to sync %s", typ)
		}
	}

	return &Fetcher{
		serverVersion: serverSemVer,
		k8s:           kubernetesFactory,
		acp:           acpFactory,
		ingressSource: ingressSource,
		crdSource:     crdSource,
	}, nil
}

// FetchState assembles a cluster state from Kubernetes resources.
func (f *Fetcher) FetchState(ctx context.Context) (*Cluster, error) {
	cluster := &Cluster{
		ExternalDNSes: make(map[string]*ExternalDNS),
	}

	var err error

	cluster.Namespaces, err = f.getNamespaces()
	if err != nil {
		return nil, err
	}

	cluster.ID, err = f.getClusterID()
	if err != nil {
		return nil, err
	}

	cluster.Apps, err = f.getApps()
	if err != nil {
		return nil, err
	}

	cluster.Services, err = f.getServices(cluster.Apps)
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

	cluster.AccessControlPolicies, err = f.getAccessControlPolicies()
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func objectKey(name, ns string) string {
	return strings.Join([]string{name, ns}, "@")
}
