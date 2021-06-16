package main

import (
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/agent"
	"github.com/traefik/hub-agent/pkg/logger"
	"github.com/traefik/hub-agent/pkg/metrics"
	"github.com/traefik/hub-agent/pkg/topology"
)

func newMetrics(watch *topology.Watcher, token, platformURL string, cfg agent.MetricsConfig) (*metrics.Manager, *metrics.Store, error) {
	rc := retryablehttp.NewClient()
	rc.RetryWaitMin = time.Second
	rc.RetryWaitMax = 10 * time.Second
	rc.RetryMax = 4
	rc.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "metrics-service").Logger())

	httpClient := rc.StandardClient()

	client, err := metrics.NewClient(httpClient, platformURL, token)
	if err != nil {
		return nil, nil, err
	}

	store := metrics.NewStore()

	scraper := metrics.NewScraper(httpClient)

	mgr := metrics.NewManager(client, store, scraper)

	mgr.SetConfig(cfg.Interval, cfg.Tables)

	watch.AddListener(mgr.TopologyStateChanged)

	return mgr, store, nil
}
