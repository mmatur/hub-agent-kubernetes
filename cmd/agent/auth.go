package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/acp/auth"
	neoclientset "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/clientset/versioned"
	neoinformer "github.com/traefik/neo-agent/pkg/crd/generated/client/neo/informers/externalversions"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/rest"
)

func authFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "auth-server.listen-addr",
			Usage:   "Address on which the auth server listens for auth requests",
			EnvVars: []string{"AUTH_SERVER_LISTEN_ADDR"},
			Value:   "0.0.0.0:80",
		},
	}
}

func runAuth(ctx context.Context, cliCtx *cli.Context) error {
	listenAddr := cliCtx.String("auth-server.listen-addr")

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	neoClientSet, err := neoclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create Neo client set: %w", err)
	}

	switcher := auth.NewHandlerSwitcher()
	acpWatcher := auth.NewWatcher(switcher)

	neoInformer := neoinformer.NewSharedInformerFactory(neoClientSet, 5*time.Minute)
	neoInformer.Neo().V1alpha1().AccessControlPolicies().Informer().AddEventHandler(acpWatcher)
	neoInformer.Start(ctx.Done())

	for t, ok := range neoInformer.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("wait for cache sync: %s: %w", t, ctx.Err())
		}
	}

	go acpWatcher.Run(ctx)

	server := &http.Server{Addr: listenAddr, Handler: switcher}
	srvDone := make(chan struct{})

	go func() {
		log.Info().Str("addr", listenAddr).Msg("Starting auth server")
		if err = server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Err(err).Msg("Unable to listen and serve auth requests")
		}
		close(srvDone)
	}()

	select {
	case <-ctx.Done():
		gracefulCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err = server.Shutdown(gracefulCtx); err != nil {
			log.Error().Err(err).Msg("Failed to shutdown auth server gracefully")
			if err = server.Close(); err != nil {
				return fmt.Errorf("close auth server: %w", err)
			}
		}
	case <-srvDone:
		return errors.New("auth server stopped")
	}

	return nil
}
