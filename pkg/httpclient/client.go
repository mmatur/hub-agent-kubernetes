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

package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/optional"
)

// Config represents configuration for an HTTP client with TLS and retry capabilities.
type Config struct {
	TLS            *ConfigTLS    `json:"tls,omitempty"`
	TimeoutSeconds *optional.Int `json:"timeoutSeconds,omitempty"`
	MaxRetries     *optional.Int `json:"maxRetries,omitempty"`
}

// ConfigTLS configures TLS for an HTTP client.
type ConfigTLS struct {
	CABundle           string `json:"caBundle,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

// New returns a new HTTP client with optional TLS and retry capabilities.
func New(cfg Config) (*http.Client, error) {
	return NewWithLogger(cfg, log.Logger)
}

// NewWithLogger returns a new HTTP client with optional TLS and retry capabilities.
// Retry attempts are logged using the given logger.
func NewWithLogger(cfg Config, l zerolog.Logger) (*http.Client, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = cfg.MaxRetries.IntOrDefault(3)
	client.HTTPClient.Timeout = time.Duration(cfg.TimeoutSeconds.IntOrDefault(5)) * time.Second
	client.Logger = logger.NewRetryableHTTPWrapper(l.With().Str("component", "http_client").Logger())

	if cfg.TLS == nil {
		return client.StandardClient(), nil
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}

	if cfg.TLS.CABundle != "" {
		if !pool.AppendCertsFromPEM([]byte(cfg.TLS.CABundle)) {
			return nil, errors.New("wrong CA bundle")
		}
	}

	client.HTTPClient.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			RootCAs:            pool,
			InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
		},
	}

	return client.StandardClient(), nil
}
