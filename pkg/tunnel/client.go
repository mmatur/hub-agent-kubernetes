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

package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/version"
)

// Client allows interacting with the tunnel service.
type Client struct {
	baseURL *url.URL
	token   string

	httpClient *http.Client
}

// NewClient creates a new client for the tunnel service.
func NewClient(baseURL, token string) (*Client, error) {
	u, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse client url: %w", err)
	}

	rc := retryablehttp.NewClient()
	rc.RetryMax = 4
	rc.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "tunnel-client").Logger())

	retryClient := rc.StandardClient()

	return &Client{
		baseURL:    u,
		token:      token,
		httpClient: retryClient,
	}, nil
}

// APIError represents an error returned by the API.
type APIError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"error"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("failed with code %d: %s", a.StatusCode, a.Message)
}

// Endpoint represents a tunnel endpoint.
type Endpoint struct {
	TunnelID       string `json:"tunnelId"`
	BrokerEndpoint string `json:"brokerEndpoint"`
}

// ListClusterTunnelEndpoints lists all tunnels the agent needs to open.
func (c *Client) ListClusterTunnelEndpoints(ctx context.Context) ([]Endpoint, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "tunnel-endpoints"))
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("failed with code %d: decode response: %w", resp.StatusCode, err)
		}

		return nil, apiErr
	}

	var tunnels []Endpoint
	if err = json.NewDecoder(resp.Body).Decode(&tunnels); err != nil {
		return nil, fmt.Errorf("decode obtain resp: %w", err)
	}

	return tunnels, nil
}
