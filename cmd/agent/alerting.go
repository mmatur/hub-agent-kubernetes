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
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/alerting"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/metrics"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
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
	retryableClient.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "alerting_client").Logger())

	httpClient := retryableClient.StandardClient()

	client, err := alerting.NewClient(httpClient, platformURL, token)
	if err != nil {
		return err
	}

	threshProc := alerting.NewThresholdProcessor(metrics.NewDataPointView(store), fetcher)

	mgr := alerting.NewManager(client,
		map[string]alerting.Processor{
			alerting.ThresholdType: threshProc,
		},
		alertRefreshInterval,
		alertSchedulerInterval,
	)

	return mgr.Run(ctx)
}
