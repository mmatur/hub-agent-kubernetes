package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/agent"
	"github.com/traefik/neo-agent/pkg/kube"
	"github.com/traefik/neo-agent/pkg/logger"
	"github.com/traefik/neo-agent/pkg/topology/store"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

func main() {
	app := &cli.App{
		Name:  "Neo agent CLI",
		Usage: "Run neo-agent service",
		Flags: allFlags(),
		Action: func(cliCtx *cli.Context) error {
			logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			kubeCfg, err := kube.InClusterConfigWithRetrier(2)
			if err != nil {
				return fmt.Errorf("create Kubernetes in-agent configuration: %w", err)
			}

			kubeClient, err := clientset.NewForConfig(kubeCfg)
			if err != nil {
				return fmt.Errorf("create Kubernetes client set: %w", err)
			}

			agentClient := agent.NewClient(cliCtx.String("platform-url"), cliCtx.String("token"))

			neoClusterID, agentCfg, err := setup(ctx, agentClient, kubeClient)
			if err != nil {
				return fmt.Errorf("setup agent: %w", err)
			}

			storeCfg := store.Config{
				TopologyConfig: agentCfg.Topology,
				Token:          cliCtx.String("token"),
			}
			topoWatch, err := newTopologyWatcher(ctx, neoClusterID, storeCfg)
			if err != nil {
				return err
			}

			group, ctx := errgroup.WithContext(ctx)

			group.Go(func() error {
				return runMetrics(ctx, topoWatch, cliCtx.String("token"), cliCtx.String("platform-url"), agentCfg.Metrics)
			})

			group.Go(func() error {
				topoWatch.Start(ctx)
				return nil
			})

			group.Go(func() error { return accessControl(ctx, cliCtx) })

			group.Go(func() error { return runAuth(ctx, cliCtx) })

			return group.Wait()
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func setup(ctx context.Context, agentClient *agent.Client, kubeClient clientset.Interface) (neoClusterID string, cfg agent.Config, err error) {
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return "", agent.Config{}, fmt.Errorf("get namespace: %w", err)
	}

	neoClusterID, err = agentClient.Link(ctx, string(ns.UID))
	if err != nil {
		return "", agent.Config{}, fmt.Errorf("link agent: %w", err)
	}

	agentCfg, err := agentClient.GetConfig(ctx)
	if err != nil {
		return "", agent.Config{}, fmt.Errorf("fetch agent config: %w", err)
	}

	return neoClusterID, agentCfg, nil
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
			Usage:    "The URL at which to reach the Neo platform API",
			EnvVars:  []string{"PLATFORM_URL"},
			Required: true,
			Hidden:   true,
		},
		&cli.StringFlag{
			Name:     "token",
			Usage:    "The token to use for Neo platform API calls",
			EnvVars:  []string{"TOKEN"},
			Required: true,
		},
	}

	flgs = append(flgs, authFlags()...)
	flgs = append(flgs, acpFlags()...)

	return flgs
}
