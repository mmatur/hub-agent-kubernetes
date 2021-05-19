package main

import (
	"context"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/traefik/neo-agent/pkg/alerting"
	"github.com/traefik/neo-agent/pkg/metrics"
)

const (
	// alertRefreshInterval is the interval to fetch configuration,
	// including alert rules.
	alertRefreshInterval = 10 * time.Minute

	// alertSchedulerInterval is the interval at which the scheduler
	// runs rule checks.
	alertSchedulerInterval = time.Minute
)

func runAlerting(ctx context.Context, token, platformURL string, store *metrics.Store) error {
	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryWaitMin = time.Second
	retryableClient.RetryWaitMax = 10 * time.Second
	retryableClient.RetryMax = 4

	httpClient := retryableClient.StandardClient()

	client, err := alerting.NewClient(httpClient, platformURL, token)
	if err != nil {
		return err
	}

	threshProc := alerting.NewThresholdProcessor(store)

	mgr := alerting.NewManager(client, map[string]alerting.Processor{
		alerting.ThresholdType: threshProc,
	})

	return mgr.Run(ctx, alertRefreshInterval, alertSchedulerInterval)
}
