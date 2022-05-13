package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ettle/strcase"
	"github.com/traefik/hub-agent-kubernetes/pkg/heartbeat"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/store"
	"github.com/traefik/hub-agent-kubernetes/pkg/version"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	pidFilePath           = "/var/run/hub-agent-kubernetes.pid"
	flagPlatformURL       = "platform-url"
	flagToken             = "token"
	flagTraefikMetricsURL = "traefik.metrics-url"
)

type controllerCmd struct {
	flags []cli.Flag
}

func newControllerCmd() controllerCmd {
	flgs := []cli.Flag{
		&cli.StringFlag{
			Name:    flagPlatformURL,
			Usage:   "The URL at which to reach the Hub platform API",
			Value:   "https://platform.hub.traefik.io/agent",
			EnvVars: []string{strcase.ToSNAKE(flagPlatformURL)},
			Hidden:  true,
		},
		&cli.StringFlag{
			Name:     flagToken,
			Usage:    "The token to use for Hub platform API calls",
			EnvVars:  []string{strcase.ToSNAKE(flagToken)},
			Required: true,
		},
		&cli.StringFlag{
			Name:     flagTraefikMetricsURL,
			Usage:    "The url used by Traefik to expose metrics",
			EnvVars:  []string{strcase.ToSNAKE(flagTraefikMetricsURL)},
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

	version.Log()

	if err := writePID(); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}

	platformURL, token := cliCtx.String(flagPlatformURL), cliCtx.String(flagToken)

	kubeCfg, err := kube.InClusterConfigWithRetrier(2)
	if err != nil {
		return fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	kubeClient, err := clientset.NewForConfig(kubeCfg)
	if err != nil {
		return fmt.Errorf("create Kubernetes client set: %w", err)
	}

	platformClient := platform.NewClient(platformURL, token)

	configWatcher := platform.NewConfigWatcher(15*time.Minute, platformClient)

	heartbeater := heartbeat.NewHeartbeater(platformClient)

	hubClusterID, agentCfg, err := setup(cliCtx.Context, platformClient, kubeClient)
	if err != nil {
		return fmt.Errorf("setup agent: %w", err)
	}

	storeCfg := store.Config{
		TopologyConfig: agentCfg.Topology,
		Token:          token,
	}
	topoFetcher, err := state.NewFetcher(cliCtx.Context, hubClusterID)
	if err != nil {
		return err
	}
	topoWatch, err := newTopologyWatcher(cliCtx.Context, topoFetcher, storeCfg)
	if err != nil {
		return err
	}

	mtrcsMgr, mtrcsStore, err := newMetrics(topoWatch, token, platformURL, cliCtx.String(flagTraefikMetricsURL), agentCfg.Metrics, configWatcher)
	if err != nil {
		return err
	}

	group, ctx := errgroup.WithContext(cliCtx.Context)

	group.Go(func() error {
		configWatcher.Run(ctx)
		return nil
	})

	group.Go(func() error {
		heartbeater.Run(ctx)
		return nil
	})

	group.Go(func() error {
		return mtrcsMgr.Run(ctx)
	})

	group.Go(func() error {
		topoWatch.Start(ctx)
		return nil
	})

	group.Go(func() error {
		return webhookAdmission(ctx, cliCtx, platformClient)
	})

	group.Go(func() error { return runAlerting(ctx, token, platformURL, mtrcsStore, topoFetcher) })

	return group.Wait()
}

func setup(ctx context.Context, c *platform.Client, kubeClient clientset.Interface) (hubClusterID string, cfg platform.Config, err error) {
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return "", platform.Config{}, fmt.Errorf("get namespace: %w", err)
	}

	hubClusterID, err = c.Link(ctx, string(ns.UID))
	if err != nil {
		return "", platform.Config{}, fmt.Errorf("link agent: %w", err)
	}

	cfg, err = c.GetConfig(ctx)
	if err != nil {
		return "", platform.Config{}, fmt.Errorf("fetch agent config: %w", err)
	}

	return hubClusterID, cfg, nil
}

func writePID() error {
	f, err := os.OpenFile(pidFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	pid := os.Getpid()
	if _, err = f.WriteString(strconv.Itoa(pid)); err != nil {
		_ = f.Close()
		return err
	}

	if err = f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}
