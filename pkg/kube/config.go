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

package kube

import (
	"fmt"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"k8s.io/client-go/rest"
)

// InClusterConfigWithRetrier returns a new in-cluster configuration that will retry requests that result in transient failures.
func InClusterConfigWithRetrier(maxRetries int) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	// We first need to get the TLS configuration since we
	// are going to bypass Kubernetes' default HTTP client.
	tlsCfg, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("create TLS config: %w", err)
	}

	rc := retryablehttp.NewClient()
	rc.RetryMax = maxRetries
	rc.HTTPClient.Transport = &http.Transport{TLSClientConfig: tlsCfg}
	rc.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "kubernetes_client").Logger())

	// By default, retryablehttp client returns an error when it reaches the maxRetry even if the doErr is nil.
	// This error prevents kubernetes library from making a clean log. This errorHandler avoids this mechanism.
	rc.ErrorHandler = func(resp *http.Response, err error, numTries int) (*http.Response, error) {
		return resp, err
	}

	rrt := &retryablehttp.RoundTripper{Client: rc}

	wt := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if wt != nil {
			wt(rt)
		}
		return rrt
	}
	return cfg, nil
}
