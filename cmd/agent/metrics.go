package main

import (
	"context"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/logger"
	"github.com/traefik/neo-agent/pkg/metrics"
	"github.com/traefik/neo-agent/pkg/topology"
	"github.com/urfave/cli/v2"
)

func runMetrics(ctx context.Context, cliCtx *cli.Context, watch *topology.Watcher) error {
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
	defer func() { _ = mgr.Close() }()

	mgr.SetConfig(time.Minute, []string{"1m", "10m", "1h", "1d"})

	watch.AddListener(mgr.TopologyStateChanged)

	return mgr.Run(ctx)
}
