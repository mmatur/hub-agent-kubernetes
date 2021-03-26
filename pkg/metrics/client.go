package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hamba/avro"
	"github.com/traefik/neo-agent/pkg/metrics/protocol"
)

// Config represents the neo agent configuration.
type Config struct {
	Interval     time.Duration               `avro:"interval"`
	Tables       []string                    `avro:"tables"`
	PreviousData map[string][]DataPointGroup `avro:"previous_data"`
}

// Client for the token service.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client

	configSchema  avro.Schema
	metricsSchema avro.Schema

	token string
}

// NewClient creates a token service client.
func NewClient(client *http.Client, baseURL, token string) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid metrics client url: %w", err)
	}

	configSchema, err := avro.Parse(protocol.ConfigV1Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid config schema: %w", err)
	}

	metricsSchema, err := avro.Parse(protocol.MetricsV1Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid metrics schema: %w", err)
	}

	return &Client{
		baseURL:       base,
		httpClient:    client,
		configSchema:  configSchema,
		metricsSchema: metricsSchema,
		token:         token,
	}, nil
}

// GetConfig gets the agent configuration.
func (c *Client) GetConfig(ctx context.Context, startup bool) (*Config, error) {
	endpoint, err := c.baseURL.Parse("/config")
	if err != nil {
		return nil, fmt.Errorf("creating metrics config url: %w", err)
	}

	qry := endpoint.Query()
	qry.Set("startup", strconv.FormatBool(startup))
	endpoint.RawQuery = qry.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Accept", "avro/binary;v1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting metrics config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("getting metrics config got %d: %s", resp.StatusCode, string(body))
	}

	var cfg Config
	if err = avro.Unmarshal(c.configSchema, body, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w: %s", err, string(body))
	}

	return &cfg, nil
}

// Send sends metrics to the metrics service.
func (c *Client) Send(ctx context.Context, data map[string][]DataPointGroup) error {
	endpoint, err := c.baseURL.Parse("/")
	if err != nil {
		return fmt.Errorf("creating metrics url: %w", err)
	}

	raw, err := avro.Marshal(c.metricsSchema, data)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "avro/binary;v1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending metrics: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sending metrics got %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
}
