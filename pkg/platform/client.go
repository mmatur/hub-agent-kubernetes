package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/logger"
)

// APIError represents an error returned by the API.
type APIError struct {
	StatusCode int
	Message    string `json:"error"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("failed with code %d: %s", a.StatusCode, a.Message)
}

// Client allows to interact with the cluster service.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new client for the cluster service.
func NewClient(baseURL, token string) *Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = 4
	rc.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "platform_client").Logger())

	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: rc.StandardClient(),
	}
}

type linkClusterReq struct {
	KubeID string `json:"kubeId"`
}

type linkClusterResp struct {
	ClusterID string `json:"clusterId"`
}

// Link links the agent to the given Kubernetes ID.
func (c *Client) Link(ctx context.Context, kubeID string) (string, error) {
	body, err := json.Marshal(linkClusterReq{KubeID: kubeID})
	if err != nil {
		return "", fmt.Errorf("marshal link agent request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/link", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			return "", errors.New("this token is already used by an agent in another Kubernetes cluster")
		}

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return "", fmt.Errorf("failed with code %d: decode response: %w", resp.StatusCode, err)
		}

		return "", apiErr
	}

	var linkResp linkClusterResp
	if err = json.NewDecoder(resp.Body).Decode(&linkResp); err != nil {
		return "", fmt.Errorf("decode link agent resp: %w", err)
	}

	return linkResp.ClusterID, nil
}

// Config holds the configuration of the offer.
type Config struct {
	Topology      TopologyConfig      `json:"topology"`
	Metrics       MetricsConfig       `json:"metrics"`
	AccessControl AccessControlConfig `json:"accessControl"`
}

// TopologyConfig holds the topology part of the offer config.
type TopologyConfig struct {
	GitProxyHost string `json:"gitProxyHost,omitempty"`
	GitOrgName   string `json:"gitOrgName,omitempty"`
	GitRepoName  string `json:"gitRepoName,omitempty"`
}

// MetricsConfig holds the metrics part of the offer config.
type MetricsConfig struct {
	Interval time.Duration `json:"interval"`
	Tables   []string      `json:"tables"`
}

// AccessControlConfig holds the configuration of the access control section of the offer config.
type AccessControlConfig struct {
	MaxSecuredRoutes int `json:"maxSecuredRoutes"`
}

// GetConfig returns the agent configuration.
func (c *Client) GetConfig(ctx context.Context) (Config, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/config", nil)
	if err != nil {
		return Config{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return Config{}, fmt.Errorf("failed with code %d: decode response: %w", resp.StatusCode, err)
		}

		return Config{}, apiErr
	}

	var cfg Config
	if err = json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}

// Ping sends a ping to the platform to inform that the agent is alive.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/ping", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed with code %d", resp.StatusCode)
	}
	return nil
}

// ReportSecuredRoutesInUse reports the number of secured routes in use to the platform.
func (c *Client) ReportSecuredRoutesInUse(ctx context.Context, n int) error {
	payload := struct {
		Name    string `json:"name"`
		Current int    `json:"current"`
	}{
		Name:    "maxSecuredRoutes",
		Current: n,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal quota usage request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/quotas", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed with code %d", resp.StatusCode)
	}
	return nil
}
