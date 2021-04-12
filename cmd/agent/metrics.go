package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/logger"
	"github.com/traefik/neo-agent/pkg/metrics"
	"github.com/urfave/cli/v2"
)

const (
	flagScrapeName   = "scrape-name"
	flagScrapeKind   = "scrape-kind"
	flagScrapeIP     = "scrape-ip"
	flagTopologyInfo = "topology-info"
)

func metricsFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     flagScrapeName,
			Usage:    "The name of the ingress controller",
			EnvVars:  []string{"SCRAPE_NAME"},
			Required: true,
		},
		&cli.StringFlag{
			Name:     flagScrapeKind,
			Usage:    "The ingress controller type to scrape (nginx, traefik or haproxy)",
			EnvVars:  []string{"SCRAPE_KIND"},
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:     flagScrapeIP,
			Usage:    "An IP of a ingress controller to scrape",
			EnvVars:  []string{"SCRAPE_IP"},
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:     flagTopologyInfo,
			Usage:    "Topology information about ingresses",
			EnvVars:  []string{"TOPOLOGY_INFO"},
			Required: true,
		},
	}
}

func runMetrics(ctx context.Context, cliCtx *cli.Context) error {
	ingressSvcs := make(map[string][]string)
	for _, ingInfo := range cliCtx.StringSlice(flagTopologyInfo) {
		parts := strings.SplitN(ingInfo, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid topology information: %s", ingInfo)
		}

		ingressSvcs[parts[0]] = strings.Split(parts[1], ",")
	}

	rc := retryablehttp.NewClient()
	rc.RetryWaitMin = time.Second
	rc.RetryWaitMax = 10 * time.Second
	rc.RetryMax = 4
	rc.Logger = logger.NewWrappedLogger(log.Logger.With().Str("component", "metrics-service").Logger())

	httpClient := rc.StandardClient()

	client, err := metrics.NewClient(httpClient, cliCtx.String("platform-url"), cliCtx.String("token"))
	if err != nil {
		return err
	}

	store := metrics.NewStore()

	scraper := metrics.NewScraper(httpClient)

	mgr := metrics.NewManager(client, store, scraper)

	return mgr.Run(ctx, time.Minute, cliCtx.String(flagScrapeKind), cliCtx.String(flagScrapeName), cliCtx.StringSlice(flagScrapeIP), ingressSvcs)
}
