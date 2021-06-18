package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acp/admission"
	"github.com/traefik/hub-agent/pkg/acp/admission/ingclass"
	"github.com/traefik/hub-agent/pkg/acp/admission/quota"
	"github.com/traefik/hub-agent/pkg/acp/admission/reviewer"
	hubclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/informers/externalversions"
	traefikclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent/pkg/kube"
	"github.com/traefik/hub-agent/pkg/kubevers"
	"github.com/traefik/hub-agent/pkg/platform"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func acpFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "acp-server.listen-addr",
			Usage:   "Address on which the access control policy server listens for admission requests",
			EnvVars: []string{"ACP_SERVER_LISTEN_ADDR"},
			Value:   "0.0.0.0:443",
		},
		&cli.StringFlag{
			Name:    "acp-server.cert",
			Usage:   "Certificate used for TLS by the ACP server",
			EnvVars: []string{"ACP_SERVER_CERT"},
			Value:   "/var/run/hub-agent/cert.pem",
		},
		&cli.StringFlag{
			Name:    "acp-server.key",
			Usage:   "Key used for TLS by the ACP server",
			EnvVars: []string{"ACP_SERVER_KEY"},
			Value:   "/var/run/hub-agent/key.pem",
		},
		&cli.StringFlag{
			Name:    "acp-server.auth-server-addr",
			Usage:   "Address the ACP server can reach the auth server on",
			EnvVars: []string{"ACP_SERVER_AUTH_SERVER_ADDR"},
			Value:   "http://hub-agent-auth-server.hub.svc.cluster.local",
		},
	}
}

func accessControl(ctx context.Context, cliCtx *cli.Context, cfg platform.AccessControlConfig) error {
	var (
		listenAddr     = cliCtx.String("acp-server.listen-addr")
		certFile       = cliCtx.String("acp-server.cert")
		keyFile        = cliCtx.String("acp-server.key")
		authServerAddr = cliCtx.String("acp-server.auth-server-addr")
	)

	if _, err := url.Parse(authServerAddr); err != nil {
		return fmt.Errorf("invalid auth server address: %w", err)
	}

	h, err := setupAdmissionHandler(ctx, authServerAddr, cfg.MaxSecuredRoutes)
	if err != nil {
		return fmt.Errorf("create admission handler: %w", err)
	}

	server := &http.Server{
		Addr:     listenAddr,
		Handler:  h,
		ErrorLog: stdlog.New(log.Logger.Level(zerolog.DebugLevel), "", 0),
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

func setupAdmissionHandler(ctx context.Context, authServerAddr string, maxSecuredRoutes int) (http.Handler, error) {
	config, err := kube.InClusterConfigWithRetrier(2)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	clientSet, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client set: %w", err)
	}

	hubClientSet, err := hubclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Hub client set: %w", err)
	}

	kubeVers, err := clientSet.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("detect Kubernetes version: %w", err)
	}

	kubeInformer := informers.NewSharedInformerFactory(clientSet, 5*time.Minute)
	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 5*time.Minute)

	updater := admission.NewIngressUpdater(kubeInformer, clientSet, kubeVers.GitVersion)

	go updater.Run(ctx)

	eventHandler := admission.NewEventHandler(updater)

	ingClassWatcher := ingclass.NewWatcher()

	err = startKubeInformer(ctx, kubeVers.GitVersion, kubeInformer, ingClassWatcher)
	if err != nil {
		return nil, fmt.Errorf("start kube informer: %w", err)
	}

	err = startHubInformer(ctx, hubInformer, eventHandler, ingClassWatcher)
	if err != nil {
		return nil, fmt.Errorf("start Hub informer: %w", err)
	}

	traefikClientSet, err := traefikclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Traefik client set: %w", err)
	}

	polGetter := reviewer.NewPolGetter(hubInformer)

	fwdAuthMdlwrs := reviewer.NewFwdAuthMiddlewares(authServerAddr, polGetter, traefikClientSet.TraefikV1alpha1())

	quotas := quota.New(maxSecuredRoutes)

	reviewers := []admission.Reviewer{
		reviewer.NewNginxIngress(authServerAddr, ingClassWatcher, polGetter, quotas),
		reviewer.NewTraefikIngress(ingClassWatcher, fwdAuthMdlwrs, quotas),
		reviewer.NewTraefikIngressRoute(fwdAuthMdlwrs, quotas),
		reviewer.NewHAProxyIngress(authServerAddr, ingClassWatcher, polGetter, quotas),
	}

	return admission.NewHandler(reviewers), nil
}

func startKubeInformer(ctx context.Context, kubeVers string, kubeInformer informers.SharedInformerFactory, ingClassEventHandler cache.ResourceEventHandler) error {
	if kubevers.SupportsIngressClasses(kubeVers) {
		kubeInformer.Networking().V1().IngressClasses().Informer().AddEventHandler(ingClassEventHandler)
	}
	if kubevers.SupportsNetV1Beta1IngressClasses(kubeVers) {
		kubeInformer.Networking().V1beta1().IngressClasses().Informer().AddEventHandler(ingClassEventHandler)
	}

	// Since we only support Kubernetes v1.14 and up,
	// we should always at least have net v1beta1 Ingresses.
	kubeInformer.Networking().V1beta1().Ingresses().Informer()
	if kubevers.SupportsNetV1Ingresses(kubeVers) {
		kubeInformer.Networking().V1().Ingresses().Informer()
	}

	kubeInformer.Start(ctx.Done())

	for t, ok := range kubeInformer.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("wait for cache Kubernetes sync: %s: %w", t, ctx.Err())
		}
	}

	return nil
}

func startHubInformer(ctx context.Context, hubInformer hubinformer.SharedInformerFactory, acpEventHandler, ingClassEventHandler cache.ResourceEventHandler) error {
	hubInformer.Hub().V1alpha1().IngressClasses().Informer().AddEventHandler(ingClassEventHandler)
	hubInformer.Hub().V1alpha1().AccessControlPolicies().Informer().AddEventHandler(acpEventHandler)

	hubInformer.Start(ctx.Done())

	for t, ok := range hubInformer.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("wait for Hub cache sync: %s: %w", t, ctx.Err())
		}
	}

	return nil
}
