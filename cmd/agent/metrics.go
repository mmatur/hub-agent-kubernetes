/*
Copyright (C) 2022 Traefik Labs

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
	"fmt"
	"net/url"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/metrics"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology"
)

func newMetrics(watch *topology.Watcher, token, platformURL, traefikURL string, cfg platform.MetricsConfig, cfgWatcher *platform.ConfigWatcher) (*metrics.Manager, *metrics.Store, error) {
	rc := retryablehttp.NewClient()
	rc.RetryWaitMin = time.Second
	rc.RetryWaitMax = 10 * time.Second
	rc.RetryMax = 4
	rc.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "metrics_client").Logger())

	httpClient := rc.StandardClient()

	client, err := metrics.NewClient(httpClient, platformURL, token)
	if err != nil {
		return nil, nil, err
	}

	u, err := url.ParseRequestURI(traefikURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse traefik metrics url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, nil, fmt.Errorf("only http and https is supported, %s found", u.Scheme)
	}

	store := metrics.NewStore()

	scraper := metrics.NewScraper(httpClient)

	mgr := metrics.NewManager(client, traefikURL, store, scraper)

	mgr.SetConfig(cfg.Interval, cfg.Tables)

	watch.AddListener(mgr.TopologyStateChanged)

	cfgWatcher.AddListener(func(cfg platform.Config) {
		mgr.SetConfig(cfg.Metrics.Interval, cfg.Metrics.Tables)
	})

	return mgr, store, nil
}
