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

package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ettle/strcase"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/ingclass"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	apiadmission "github.com/traefik/hub-agent-kubernetes/pkg/api/admission"
	apireviewer "github.com/traefik/hub-agent-kubernetes/pkg/api/admission/reviewer"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformers "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/typed/traefik/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	edgeadmission "github.com/traefik/hub-agent-kubernetes/pkg/edgeingress/admission"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	"github.com/traefik/hub-agent-kubernetes/pkg/kubevers"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	"github.com/urfave/cli/v2"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	kinformers "k8s.io/client-go/informers"
	kclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/strings/slices"
)

const (
	flagACPServerListenAddr               = "acp-server.listen-addr"
	flagACPServerCertificate              = "acp-server.cert"
	flagACPServerKey                      = "acp-server.key"
	flagACPServerAuthServerAddr           = "acp-server.auth-server-addr"
	flagIngressClassName                  = "ingress-class-name"
	flagTraefikAPIEntryPoint              = "traefik.api.entryPoint"
	flagTraefikTunnelEntryPoint           = "traefik.tunnel.entryPoint"
	flagTraefikTunnelEntryPointDeprecated = "traefik.entryPoint"
	flagDevPortalServiceName              = "dev-portal.service-name"
	flagDevPortalPort                     = "dev-portal.port"
)

const apiManagementFeature = "api-management"

func devPortalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    flagDevPortalServiceName,
			Usage:   "Service name of the Dev Portal",
			EnvVars: []string{strcase.ToSNAKE(flagACPServerAuthServerAddr)},
			Value:   "hub-agent-dev-portal",
		},
		&cli.IntFlag{
			Name:    flagDevPortalPort,
			Usage:   "Port used by the Dev Portal service",
			EnvVars: []string{strcase.ToSNAKE(flagACPServerAuthServerAddr)},
			Value:   80,
		},
	}
}

func admissionFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    flagACPServerListenAddr,
			Usage:   "Address on which the access control policy server listens for admission requests",
			EnvVars: []string{strcase.ToSNAKE(flagACPServerListenAddr)},
			Value:   "0.0.0.0:443",
		},
		&cli.StringFlag{
			Name:    flagACPServerCertificate,
			Usage:   "Certificate used for TLS by the ACP server",
			EnvVars: []string{strcase.ToSNAKE(flagACPServerCertificate)},
			Value:   "/var/run/hub-agent-kubernetes/cert.pem",
		},
		&cli.StringFlag{
			Name:    flagACPServerKey,
			Usage:   "Key used for TLS by the ACP server",
			EnvVars: []string{strcase.ToSNAKE(flagACPServerKey)},
			Value:   "/var/run/hub-agent-kubernetes/key.pem",
		},
		&cli.StringFlag{
			Name:    flagACPServerAuthServerAddr,
			Usage:   "Address the ACP server can reach the auth server on",
			EnvVars: []string{strcase.ToSNAKE(flagACPServerAuthServerAddr)},
			Value:   "http://hub-agent-auth-server.hub.svc.cluster.local",
		},
		&cli.StringFlag{
			Name:    flagIngressClassName,
			Usage:   "The ingress class name used for ingresses managed by Hub",
			EnvVars: []string{strcase.ToSNAKE(flagIngressClassName)},
			Value:   "traefik-hub",
		},
		&cli.StringFlag{
			Name:    flagTraefikAPIEntryPoint,
			Usage:   "The entry point used by Traefik to expose APIs",
			EnvVars: []string{strcase.ToSNAKE(flagTraefikAPIEntryPoint)},
			Value:   "websecure",
		},
		&cli.StringFlag{
			Name:    flagTraefikTunnelEntryPoint,
			Usage:   "The entry point used by Traefik to expose tunnels",
			EnvVars: []string{strcase.ToSNAKE(flagTraefikTunnelEntryPoint)},
			Value:   "traefikhub-tunl",
		},
		&cli.StringFlag{
			Name:    flagTraefikTunnelEntryPointDeprecated,
			Usage:   fmt.Sprintf("Deprecated - Please use --%s instead", flagTraefikTunnelEntryPoint),
			EnvVars: []string{strcase.ToSNAKE(flagTraefikTunnelEntryPointDeprecated)},
			Value:   "traefikhub-tunl",
		},
	}
}

func webhookAdmission(ctx context.Context, cliCtx *cli.Context, platformClient *platform.Client, cfgWatcher *platform.ConfigWatcher) error {
	var (
		listenAddr     = cliCtx.String(flagACPServerListenAddr)
		certFile       = cliCtx.String(flagACPServerCertificate)
		keyFile        = cliCtx.String(flagACPServerKey)
		authServerAddr = cliCtx.String(flagACPServerAuthServerAddr)
	)

	// Handle --traefik.entryPoint deprecation.
	traefikTunnelEntrypoint := cliCtx.String(flagTraefikTunnelEntryPoint)
	if traefikTunnelEntrypoint == "" {
		traefikTunnelEntrypoint = cliCtx.String(flagTraefikTunnelEntryPointDeprecated)
	}

	if _, err := url.Parse(authServerAddr); err != nil {
		return fmt.Errorf("invalid auth server address: %w", err)
	}

	edgeIngressWatcherCfg := edgeingress.WatcherConfig{
		IngressClassName:        cliCtx.String(flagIngressClassName),
		TraefikTunnelEntryPoint: traefikTunnelEntrypoint,
		AgentNamespace:          currentNamespace(),
		EdgeIngressSyncInterval: time.Minute,
		CertRetryInterval:       time.Minute,
		CertSyncInterval:        time.Hour,
	}

	portalWatcherCfg := &api.WatcherPortalConfig{
		IngressClassName:            cliCtx.String(flagIngressClassName),
		AgentNamespace:              currentNamespace(),
		TraefikAPIEntryPoint:        cliCtx.String(flagTraefikAPIEntryPoint),
		TraefikTunnelEntryPoint:     cliCtx.String(flagTraefikTunnelEntryPoint),
		DevPortalServiceName:        cliCtx.String(flagDevPortalServiceName),
		DevPortalPort:               cliCtx.Int(flagDevPortalPort),
		PlatformIdentityProviderURL: cliCtx.String(flagPlatformIdentityProviderURL),
		PortalSyncInterval:          time.Minute,
		CertSyncInterval:            time.Hour,
		CertRetryInterval:           time.Minute,
	}

	gatewayWatcherCfg := &api.WatcherGatewayConfig{
		IngressClassName:        cliCtx.String(flagIngressClassName),
		AgentNamespace:          currentNamespace(),
		TraefikAPIEntryPoint:    cliCtx.String(flagTraefikAPIEntryPoint),
		TraefikTunnelEntryPoint: cliCtx.String(flagTraefikTunnelEntryPoint),
		GatewaySyncInterval:     time.Minute,
		CertSyncInterval:        time.Hour,
		CertRetryInterval:       time.Minute,
	}

	acpAdmission, edgeIngressAdmission, apiAdmission, err := setupAdmissionHandlers(ctx, platformClient, authServerAddr, edgeIngressWatcherCfg, portalWatcherCfg, gatewayWatcherCfg, cfgWatcher)
	if err != nil {
		return fmt.Errorf("create admission handler: %w", err)
	}

	webAdmissionACP := admission.NewACPHandler(platformClient)

	router := chi.NewRouter()
	router.Handle("/edge-ingress", edgeIngressAdmission)
	if apiAdmission != nil {
		router.Handle("/api", apiAdmission)
		router.Handle("/api-collection", apiAdmission)
		router.Handle("/api-access", apiAdmission)
		router.Handle("/api-gateway", apiAdmission)
		router.Handle("/api-portal", apiAdmission)
	}
	router.Handle("/ingress", acpAdmission)
	router.Handle("/acp", webAdmissionACP)

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ErrorLog:          stdlog.New(log.Logger.Level(zerolog.DebugLevel), "", 0),
		ReadHeaderTimeout: 2 * time.Second,
	}
	srvDone := make(chan struct{})

	go func() {
		log.Info().Str("addr", listenAddr).Msg("Starting admission server")
		if err = server.ListenAndServeTLS(certFile, keyFile); !errors.Is(err, http.ErrServerClosed) {
			log.Err(err).Msg("Unable to listen and serve admission requests")
		}
		close(srvDone)
	}()

	select {
	case <-ctx.Done():
		gracefulCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err = server.Shutdown(gracefulCtx); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown admission server gracefully")
			if err = server.Close(); err != nil {
				return fmt.Errorf("close admission server: %w", err)
			}
		}
		log.Info().Msg("Successfully shutdown admission server")
	case <-srvDone:
		return errors.New("admission server stopped")
	}

	return nil
}

func setupAdmissionHandlers(ctx context.Context, platformClient *platform.Client, authServerAddr string, edgeIngressWatcherCfg edgeingress.WatcherConfig, portalWatcherCfg *api.WatcherPortalConfig, gatewayWatcherCfg *api.WatcherGatewayConfig, cfgWatcher *platform.ConfigWatcher) (acpHandler, edgeIngressHandler, apiHandler http.Handler, err error) {
	config, err := kube.InClusterConfigWithRetrier(2)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	kubeClientSet, err := kclientset.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create Kubernetes client set: %w", err)
	}

	if err = initIngressClass(ctx, kubeClientSet, edgeIngressWatcherCfg.IngressClassName); err != nil {
		return nil, nil, nil, fmt.Errorf("initialize ingressClass: %w", err)
	}

	hubClientSet, err := hubclientset.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create Hub client set: %w", err)
	}
	traefikClientSet, err := createTraefikClientSet(kubeClientSet, config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create Traefik client set: %w", err)
	}

	kubeVers, err := kubeClientSet.Discovery().ServerVersion()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("detect Kubernetes version: %w", err)
	}

	kubeInformer := kinformers.NewSharedInformerFactory(kubeClientSet, 5*time.Minute)
	hubInformer := hubinformers.NewSharedInformerFactory(hubClientSet, 5*time.Minute)

	ingressUpdater := admission.NewIngressUpdater(kubeInformer, kubeClientSet, kubeVers.GitVersion)

	acpEventHandler := admission.NewEventHandler(ingressUpdater)
	ingClassWatcher := ingclass.NewWatcher()

	err = startKubeInformer(ctx, kubeVers.GitVersion, kubeInformer, ingClassWatcher)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("start kube informer: %w", err)
	}

	isAPIManagementCRDsAvailable, err := hasAPIManagementCRDs(kubeClientSet)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("API available: %w", err)
	}

	err = startHubInformer(ctx, hubInformer, ingClassWatcher, acpEventHandler, isAPIManagementCRDsAvailable)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("start kube informer: %w", err)
	}

	acpWatcher := acp.NewWatcher(time.Minute, platformClient, hubClientSet, hubInformer)

	edgeIngressWatcher, err := edgeingress.NewWatcher(platformClient, hubClientSet, kubeClientSet, traefikClientSet, hubInformer, edgeIngressWatcherCfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create edge ingress watcher: %w", err)
	}

	go acpWatcher.Run(ctx)
	go ingressUpdater.Run(ctx)
	go edgeIngressWatcher.Run(ctx)

	if isAPIManagementCRDsAvailable {
		if err = setupAPIManagementWatcher(ctx,
			platformClient, kubeClientSet, hubClientSet,
			traefikClientSet, kubeInformer, hubInformer,
			portalWatcherCfg, gatewayWatcherCfg, cfgWatcher); err != nil {
			return nil, nil, nil, fmt.Errorf("setup API management watcher: %w", err)
		}
	}

	polGetter := reviewer.NewPolGetter(hubInformer)

	fwdAuthMdlwrs := reviewer.NewFwdAuthMiddlewares(authServerAddr, polGetter, traefikClientSet)

	traefikReviewer := reviewer.NewTraefikIngress(ingClassWatcher, fwdAuthMdlwrs)
	reviewers := []admission.Reviewer{
		reviewer.NewNginxIngress(authServerAddr, ingClassWatcher, polGetter),
		reviewer.NewTraefikIngressRoute(fwdAuthMdlwrs),
		traefikReviewer,
	}

	if isAPIManagementCRDsAvailable {
		rev := []apiadmission.Reviewer{
			apireviewer.NewAPI(platformClient),
			apireviewer.NewCollection(platformClient),
			apireviewer.NewAccess(platformClient),
			apireviewer.NewPortal(platformClient),
			apireviewer.NewGateway(platformClient),
		}
		apiHandler = apiadmission.NewHandler(rev)
	}

	return admission.NewHandler(reviewers, traefikReviewer), edgeadmission.NewHandler(platformClient), apiHandler, nil
}

func setupAPIManagementWatcher(
	ctx context.Context,
	platformClient *platform.Client,
	kubeClientSet *kclientset.Clientset,
	hubClientSet *hubclientset.Clientset,
	traefikClientSet v1alpha1.TraefikV1alpha1Interface,
	kubeInformer kinformers.SharedInformerFactory,
	hubInformer hubinformers.SharedInformerFactory,
	portalWatcherCfg *api.WatcherPortalConfig,
	gatewayWatcherCfg *api.WatcherGatewayConfig,
	cfgWatcher *platform.ConfigWatcher,
) error {
	portalWatcher := api.NewWatcherPortal(platformClient, kubeClientSet, kubeInformer, hubClientSet, hubInformer, portalWatcherCfg)
	gatewayWatcher := api.NewWatcherGateway(platformClient, kubeClientSet, kubeInformer, hubClientSet, hubInformer, traefikClientSet, gatewayWatcherCfg)
	apiWatcher := api.NewWatcherAPI(platformClient, hubClientSet, hubInformer, portalWatcherCfg.PortalSyncInterval)
	collectionWatcher := api.NewWatcherCollection(platformClient, hubClientSet, hubInformer, portalWatcherCfg.PortalSyncInterval)
	accessWatcher := api.NewWatcherAccess(platformClient, hubClientSet, hubInformer, portalWatcherCfg.PortalSyncInterval)

	var cancel func()
	var watcherStarted bool
	startWatchers := func(ctx context.Context) {
		var apiCtx context.Context
		apiCtx, cancel = context.WithCancel(ctx)

		go portalWatcher.Run(apiCtx)
		go gatewayWatcher.Run(apiCtx)
		go apiWatcher.Run(apiCtx)
		go collectionWatcher.Run(apiCtx)
		go accessWatcher.Run(apiCtx)

		watcherStarted = true
	}

	stopWatchers := func() {
		cancel()
		watcherStarted = false
	}

	cfg, err := platformClient.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}

	if slices.Contains(cfg.Features, apiManagementFeature) {
		startWatchers(ctx)
	}

	cfgWatcher.AddListener(func(cfg platform.Config) {
		if slices.Contains(cfg.Features, apiManagementFeature) {
			if !watcherStarted {
				startWatchers(ctx)
			}
		} else if watcherStarted {
			stopWatchers()
		}
	})

	return nil
}

func createTraefikClientSet(clientSet *kclientset.Clientset, config *rest.Config) (v1alpha1.TraefikV1alpha1Interface, error) {
	crd, err := hasMiddlewareCRD(clientSet.Discovery())
	if err != nil {
		return nil, fmt.Errorf("check presence of Traefik Middleware CRD: %w", err)
	}

	if !crd {
		return nil, nil
	}

	traefikClientSet, errClientSet := traefikclientset.NewForConfig(config)
	if errClientSet != nil {
		return nil, fmt.Errorf("create Traefik client set: %w", errClientSet)
	}

	return traefikClientSet.TraefikV1alpha1(), nil
}

func startHubInformer(ctx context.Context, hubInformer hubinformers.SharedInformerFactory, ingClassWatcher, acpEventHandler cache.ResourceEventHandler, apiAvailable bool) error {
	if _, err := hubInformer.Hub().V1alpha1().IngressClasses().Informer().AddEventHandler(ingClassWatcher); err != nil {
		return fmt.Errorf("add ingressClass event handler: %w", err)
	}

	if _, err := hubInformer.Hub().V1alpha1().AccessControlPolicies().Informer().AddEventHandler(acpEventHandler); err != nil {
		return fmt.Errorf("add accessControlPolicy event handler: %w", err)
	}

	hubInformer.Hub().V1alpha1().EdgeIngresses().Informer()

	if apiAvailable {
		hubInformer.Hub().V1alpha1().APIAccesses().Informer()
		hubInformer.Hub().V1alpha1().APIPortals().Informer()
		hubInformer.Hub().V1alpha1().APIGateways().Informer()
		hubInformer.Hub().V1alpha1().APICollections().Informer()
		hubInformer.Hub().V1alpha1().APIs().Informer()
	}

	hubInformer.Start(ctx.Done())

	for t, ok := range hubInformer.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("wait for Hub informer cache sync: %s: %w", t, ctx.Err())
		}
	}

	return nil
}

func startKubeInformer(ctx context.Context, kubeVers string, kubeInformer kinformers.SharedInformerFactory, ingClassEventHandler cache.ResourceEventHandler) error {
	if kubevers.SupportsNetV1IngressClasses(kubeVers) {
		if _, err := kubeInformer.Networking().V1().IngressClasses().Informer().AddEventHandler(ingClassEventHandler); err != nil {
			return fmt.Errorf("add v1 IngressClass event handler: %w", err)
		}
	} else if kubevers.SupportsNetV1Beta1IngressClasses(kubeVers) {
		if _, err := kubeInformer.Networking().V1beta1().IngressClasses().Informer().AddEventHandler(ingClassEventHandler); err != nil {
			return fmt.Errorf("add v1beta1 IngressClass event handler: %w", err)
		}
	}

	if kubevers.SupportsNetV1Ingresses(kubeVers) {
		kubeInformer.Networking().V1().Ingresses().Informer()
	} else {
		// Since we only support Kubernetes v1.14 and up, we should always at least have net v1beta1 Ingresses.
		kubeInformer.Networking().V1beta1().Ingresses().Informer()
	}

	kubeInformer.Start(ctx.Done())

	for t, ok := range kubeInformer.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("wait for informer cache sync: %s: %w", t, ctx.Err())
		}
	}

	return nil
}

func initIngressClass(ctx context.Context, clientSet kclientset.Interface, ingressClassName string) error {
	ic := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressClassName,
		},
		Spec: netv1.IngressClassSpec{
			Controller: "traefik.io/ingress-controller",
		},
	}
	if _, err := clientSet.NetworkingV1().IngressClasses().Create(ctx, ic, metav1.CreateOptions{}); err != nil {
		if !kerror.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

func currentNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}

	return "default"
}

func hasMiddlewareCRD(clientSet discovery.DiscoveryInterface) (bool, error) {
	crdList, err := clientSet.ServerResourcesForGroupVersion(traefikv1alpha1.SchemeGroupVersion.String())
	if err != nil {
		if kerror.IsNotFound(err) ||
			// Because the fake client doesn't return the right error type.
			strings.HasSuffix(err.Error(), " not found") {
			return false, nil
		}
		return false, err
	}

	for _, resource := range crdList.APIResources {
		if resource.Kind == "Middleware" {
			return true, nil
		}
	}

	return true, nil
}

func hasAPIManagementCRDs(clientSet discovery.DiscoveryInterface) (bool, error) {
	crdList, err := clientSet.ServerResourcesForGroupVersion(hubv1alpha1.SchemeGroupVersion.String())
	if err != nil {
		if kerror.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	log.Info().Interface("crds", crdList.APIResources).Msg("crds list")

	for _, resource := range crdList.APIResources {
		if resource.Kind == "APIPortal" {
			return true, nil
		}
	}

	return false, nil
}
