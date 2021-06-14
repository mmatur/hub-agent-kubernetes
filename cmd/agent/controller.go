package main

import (
	"context"
	"fmt"

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

type controllerCmd struct {
	flags []cli.Flag
}

func newControllerCmd() controllerCmd {
	flgs := []cli.Flag{
		&cli.StringFlag{
			Name:     "platform-url",
			Usage:    "The URL at which to reach the Hub platform API",
			Value:    "https://platform.hub.traefik.io/agent",
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

	flgs = append(flgs, globalFlags()...)
	flgs = append(flgs, acpFlags()...)

	return controllerCmd{
		flags: flgs,
	}
}

func (c controllerCmd) build() *cli.Command {
	return &cli.Command{
		Name:   "controller",
		Usage:  "Runs the Hub agent controller",
		Flags:  c.flags,
		Action: c.run,
	}
}

func (c controllerCmd) run(cliCtx *cli.Context) error {
	logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

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

	hubClusterID, agentCfg, err := setup(cliCtx.Context, agentClient, kubeClient)
	if err != nil {
		return fmt.Errorf("setup agent: %w", err)
	}

	storeCfg := store.Config{
		TopologyConfig: agentCfg.Topology,
		Token:          cliCtx.String("token"),
	}
	topoFetcher, err := state.NewFetcher(cliCtx.Context, hubClusterID)
	if err != nil {
		return err
	}
	topoWatch, err := newTopologyWatcher(cliCtx.Context, topoFetcher, storeCfg)
	if err != nil {
		return err
	}

	mtrcsMgr, mtrcsStore, err := newMetrics(topoWatch, token, platformURL, agentCfg.Metrics)
	if err != nil {
		return err
	}
	defer func() { _ = mtrcsMgr.Close() }()

	group, ctx := errgroup.WithContext(cliCtx.Context)

	group.Go(func() error {
		return mtrcsMgr.Run(ctx)
	})

	group.Go(func() error {
		topoWatch.Start(ctx)
		return nil
	})

	group.Go(func() error { return accessControl(ctx, cliCtx) })

	group.Go(func() error { return runAlerting(ctx, token, platformURL, mtrcsStore, topoFetcher) })

	group.Go(func() error { return runACME(ctx, platformURL, token) })

	return group.Wait()
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
