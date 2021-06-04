package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/agent"
	"github.com/traefik/hub-agent/pkg/kube"
	"github.com/traefik/hub-agent/pkg/logger"
	"github.com/traefik/hub-agent/pkg/topology/state"
	"github.com/traefik/hub-agent/pkg/topology/store"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

func main() {
	app := &cli.App{
		Name:  "Hub agent CLI",
		Usage: "Run hub-agent service",
		Flags: allFlags(),
		Action: func(cliCtx *cli.Context) error {
			logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			platformURL, token := cliCtx.String("platform-url"), cliCtx.String("token")

			kubeCfg, err := kube.InClusterConfigWithRetrier(2)
			if err != nil {
				return fmt.Errorf("create Kubernetes in-agent configuration: %w", err)
			}

			kubeClient, err := clientset.NewForConfig(kubeCfg)
			if err != nil {
				return fmt.Errorf("create Kubernetes client set: %w", err)
			}

			agentClient := agent.NewClient(platformURL, token)

			hubClusterID, agentCfg, err := setup(ctx, agentClient, kubeClient)
			if err != nil {
				return fmt.Errorf("setup agent: %w", err)
			}

			storeCfg := store.Config{
				TopologyConfig: agentCfg.Topology,
				Token:          cliCtx.String("token"),
			}
			topoFetcher, err := state.NewFetcher(ctx, hubClusterID)
			if err != nil {
				return err
			}
			topoWatch, err := newTopologyWatcher(ctx, topoFetcher, storeCfg)
			if err != nil {
				return err
			}

			mtrcsMgr, mtrcsStore, err := newMetrics(topoWatch, token, platformURL, agentCfg.Metrics)
			if err != nil {
				return err
			}
			defer func() { _ = mtrcsMgr.Close() }()

			group, ctx := errgroup.WithContext(ctx)

			group.Go(func() error {
				return mtrcsMgr.Run(ctx)
			})

			group.Go(func() error {
				topoWatch.Start(ctx)
				return nil
			})

			group.Go(func() error { return accessControl(ctx, cliCtx) })

			group.Go(func() error { return runAuth(ctx, cliCtx) })

			group.Go(func() error { return runAlerting(ctx, token, platformURL, mtrcsStore, topoFetcher) })

			group.Go(func() error { return runACME(ctx, platformURL, token) })

			return group.Wait()
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func setup(ctx context.Context, agentClient *agent.Client, kubeClient clientset.Interface) (hubClusterID string, cfg agent.Config, err error) {
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return "", agent.Config{}, fmt.Errorf("get namespace: %w", err)
	}

	hubClusterID, err = agentClient.Link(ctx, string(ns.UID))
	if err != nil {
		return "", agent.Config{}, fmt.Errorf("link agent: %w", err)
	}

	agentCfg, err := agentClient.GetConfig(ctx)
	if err != nil {
		return "", agent.Config{}, fmt.Errorf("fetch agent config: %w", err)
	}

	return hubClusterID, agentCfg, nil
}

func allFlags() []cli.Flag {
	flgs := []cli.Flag{
		&cli.StringFlag{
			Name:    "log-level",
			Usage:   "Log level to use (debug, info, warn, error or fatal)",
			EnvVars: []string{"LOG_LEVEL"},
			Value:   "info",
		},
		&cli.StringFlag{
			Name:    "log-format",
			Usage:   "Log format to use (json or console)",
			EnvVars: []string{"LOG_FORMAT"},
			Value:   "json",
			Hidden:  true,
		},
		&cli.StringFlag{
			Name:     "platform-url",
			Usage:    "The URL at which to reach the Hub platform API",
			EnvVars:  []string{"PLATFORM_URL"},
			Required: true,
			Hidden:   true,
		},
		&cli.StringFlag{
			Name:     "token",
			Usage:    "The token to use for Hub platform API calls",
			EnvVars:  []string{"TOKEN"},
			Required: true,
		},
	}

	flgs = append(flgs, authFlags()...)
	flgs = append(flgs, acpFlags()...)

	return flgs
}
