package main

import (
	"context"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/alerting"
	"github.com/traefik/hub-agent/pkg/logger"
	"github.com/traefik/hub-agent/pkg/metrics"
	"github.com/traefik/hub-agent/pkg/topology/state"
)

const (
	// alertRefreshInterval is the interval to fetch configuration,
	// including alert rules.
	alertRefreshInterval = 10 * time.Minute

	// alertSchedulerInterval is the interval at which the scheduler
	// runs rule checks.
	alertSchedulerInterval = time.Minute
)

func runAlerting(ctx context.Context, token, platformURL string, store *metrics.Store, fetcher *state.Fetcher) error {
	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryWaitMin = time.Second
	retryableClient.RetryWaitMax = 10 * time.Second
	retryableClient.RetryMax = 4
	retryableClient.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "alerting-service").Logger())

	httpClient := retryableClient.StandardClient()

	client, err := alerting.NewClient(httpClient, platformURL, token)
	if err != nil {
		return err
	}

	threshProc := alerting.NewThresholdProcessor(store, fetcher)

	mgr := alerting.NewManager(client, map[string]alerting.Processor{
		alerting.ThresholdType: threshProc,
	})

	return mgr.Run(ctx, alertRefreshInterval, alertSchedulerInterval)
}
