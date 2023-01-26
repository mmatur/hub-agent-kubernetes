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
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ettle/strcase"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/commands"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent-kubernetes/pkg/heartbeat"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/store"
	"github.com/traefik/hub-agent-kubernetes/pkg/version"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
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
			Name:    flagTraefikMetricsURL,
			Usage:   "The url used by Traefik to expose metrics",
			EnvVars: []string{strcase.ToSNAKE(flagTraefikMetricsURL)},
		},
	}

	flgs = append(flgs, globalFlags()...)
	flgs = append(flgs, admissionFlags()...)
	flgs = append(flgs, devPortalFlags()...)

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
	logger.Setup(cliCtx.String(flagLogLevel), cliCtx.String(flagLogFormat))

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

	if err = setupOIDCSecret(cliCtx, kubeClient, token); err != nil {
		return fmt.Errorf("setup OIDC secret: %w", err)
	}

	traefikClientSet, err := traefikclientset.NewForConfig(kubeCfg)
	if err != nil {
		return fmt.Errorf("create Traefik client set: %w", err)
	}

	hubClientSet, err := hubclientset.NewForConfig(kubeCfg)
	if err != nil {
		return fmt.Errorf("create Traefik Hub client set: %w", err)
	}

	platformClient, err := platform.NewClient(platformURL, token)
	if err != nil {
		return fmt.Errorf("build platform client: %w", err)
	}

	configWatcher := platform.NewConfigWatcher(15*time.Minute, platformClient)

	heartbeater := heartbeat.NewHeartbeater(platformClient)

	agentCfg, err := setup(cliCtx.Context, platformClient, kubeClient)
	if err != nil {
		return fmt.Errorf("setup agent: %w", err)
	}

	topoFetcher, err := state.NewFetcher(cliCtx.Context, kubeClient, traefikClientSet, hubClientSet)
	if err != nil {
		return err
	}
	topoWatch := topology.NewWatcher(topoFetcher, store.New(platformClient))

	checker := version.NewChecker(platformClient)

	commandWatcher := commands.NewWatcher(10*time.Second, platformClient, kubeClient, traefikClientSet)

	group, ctx := errgroup.WithContext(cliCtx.Context)

	group.Go(func() error {
		configWatcher.Run(ctx)
		return nil
	})

	group.Go(func() error {
		heartbeater.Run(ctx)
		return nil
	})

	if cliCtx.String(flagTraefikMetricsURL) != "" {
		mtrcsMgr, mtrcsStore, err := newMetrics(topoWatch, token, platformURL, cliCtx.String(flagTraefikMetricsURL), agentCfg.Metrics, configWatcher)
		if err != nil {
			return err
		}

		group.Go(func() error {
			return mtrcsMgr.Run(ctx)
		})

		group.Go(func() error { return runAlerting(ctx, token, platformURL, mtrcsStore, topoFetcher) })
	}

	group.Go(func() error {
		topoWatch.Start(ctx)
		return nil
	})

	group.Go(func() error {
		return webhookAdmission(ctx, cliCtx, platformClient, topoWatch)
	})

	group.Go(func() error {
		if err := checker.Start(ctx); err != nil {
			return err
		}

		return nil
	})

	group.Go(func() error {
		commandWatcher.Start(ctx)
		return nil
	})

	return group.Wait()
}

func setupOIDCSecret(cliCtx *cli.Context, client clientset.Interface, token string) error {
	ctx, cancel := context.WithTimeout(cliCtx.Context, time.Second*5)
	defer cancel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub-secret",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "traefik-hub",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key": []byte(token),
		},
	}

	if _, err := client.CoreV1().Secrets(currentNamespace()).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if kerror.IsAlreadyExists(err) {
			log.Ctx(ctx).Debug().Msg("hub secret is already here")

			return nil
		}

		return fmt.Errorf("create secret: %w", err)
	}

	return nil
}

func setup(ctx context.Context, c *platform.Client, kubeClient clientset.Interface) (platform.Config, error) {
	ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return platform.Config{}, fmt.Errorf("get namespace: %w", err)
	}

	_, err = c.Link(ctx, string(ns.UID))
	if err != nil {
		return platform.Config{}, fmt.Errorf("link agent: %w", err)
	}

	cfg, err := c.GetConfig(ctx)
	if err != nil {
		return platform.Config{}, fmt.Errorf("fetch agent config: %w", err)
	}

	return cfg, nil
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
