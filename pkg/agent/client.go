package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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

type linkClusterReq struct {
	KubeID string `json:"kubeId"`
}

type linkClusterResp struct {
	ClusterID string `json:"clusterId"`
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
	rc.Logger = logger.NewWrappedLogger(log.Logger.With().Str("component", "agent-client").Logger())

	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: rc.StandardClient(),
	}
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
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return "", fmt.Errorf("failed with code %d: decode response: %w", resp.StatusCode, err)
		}

		return "", apiErr
	}

	var linkResp linkClusterResp
	if err := json.NewDecoder(resp.Body).Decode(&linkResp); err != nil {
		return "", fmt.Errorf("decode link agent resp: %w", err)
	}

	return linkResp.ClusterID, nil
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
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}
