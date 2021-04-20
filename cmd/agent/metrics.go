package main

import (
	"context"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/agent"
	"github.com/traefik/neo-agent/pkg/logger"
	"github.com/traefik/neo-agent/pkg/metrics"
	"github.com/traefik/neo-agent/pkg/topology"
)

func runMetrics(ctx context.Context, watch *topology.Watcher, token, platformURL string, cfg agent.MetricsConfig) error {
	rc := retryablehttp.NewClient()
	rc.RetryWaitMin = time.Second
	rc.RetryWaitMax = 10 * time.Second
	rc.RetryMax = 4
	rc.Logger = logger.NewWrappedLogger(log.Logger.With().Str("component", "metrics-service").Logger())

	httpClient := rc.StandardClient()

	client, err := metrics.NewClient(httpClient, platformURL, token)
	if err != nil {
		return err
	}

	store := metrics.NewStore()

	scraper := metrics.NewScraper(httpClient)

	mgr := metrics.NewManager(client, store, scraper)
	defer func() { _ = mgr.Close() }()

	mgr.SetConfig(cfg.Interval, cfg.Tables)

	watch.AddListener(mgr.TopologyStateChanged)

	return mgr.Run(ctx)
}
