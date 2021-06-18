package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acp/auth"
	hubclientset "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/informers/externalversions"
	"github.com/traefik/hub-agent/pkg/logger"
	"github.com/urfave/cli/v2"
	"k8s.io/client-go/rest"
)

type authServerCmd struct {
	flags []cli.Flag
}

func newAuthServerCmd() authServerCmd {
	flgs := []cli.Flag{
		&cli.StringFlag{
			Name:    "listen-addr",
			Usage:   "Address on which the auth server listens for auth requests",
			EnvVars: []string{"AUTH_SERVER_LISTEN_ADDR"},
			Value:   "0.0.0.0:80",
		},
	}

	flgs = append(flgs, globalFlags()...)

	return authServerCmd{
		flags: flgs,
	}
}

func (c authServerCmd) build() *cli.Command {
	return &cli.Command{
		Name:   "auth-server",
		Usage:  "Runs the Hub agent authentication server",
		Flags:  c.flags,
		Action: c.run,
	}
}

func (c authServerCmd) run(cliCtx *cli.Context) error {
	logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	hubClientSet, err := hubclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("create Hub client set: %w", err)
	}

	switcher := auth.NewHandlerSwitcher()
	acpWatcher := auth.NewWatcher(switcher)

	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 5*time.Minute)
	hubInformer.Hub().V1alpha1().AccessControlPolicies().Informer().AddEventHandler(acpWatcher)
	hubInformer.Start(cliCtx.Context.Done())

	for t, ok := range hubInformer.WaitForCacheSync(cliCtx.Context.Done()) {
		if !ok {
			return fmt.Errorf("wait for cache sync: %s: %w", t, cliCtx.Context.Err())
		}
	}

	go acpWatcher.Run(cliCtx.Context)

	listenAddr := cliCtx.String("listen-addr")
	server := &http.Server{
		Addr:     listenAddr,
		Handler:  switcher,
		ErrorLog: stdlog.New(log.Logger.Level(zerolog.DebugLevel), "", 0),
	}

	srvDone := make(chan struct{})

	go func() {
		log.Info().Str("addr", listenAddr).Msg("Starting auth server")
		if err = server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Err(err).Msg("Unable to listen and serve auth requests")
		}
		close(srvDone)
	}()

	select {
	case <-cliCtx.Context.Done():
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
