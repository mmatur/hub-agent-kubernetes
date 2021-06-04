package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/hamba/avro"
	"github.com/traefik/hub-agent/pkg/metrics/protocol"
)

// Client for the token service.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client

	metricsSchema avro.Schema

	token string
}

// NewClient creates a token service client.
func NewClient(client *http.Client, baseURL, token string) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid metrics client url: %w", err)
	}

	metricsSchema, err := avro.Parse(protocol.MetricsV1Schema)
	if err != nil {
		return nil, fmt.Errorf("invalid metrics schema: %w", err)
	}

	return &Client{
		baseURL:       base,
		httpClient:    client,
		metricsSchema: metricsSchema,
		token:         token,
	}, nil
}

// GetPreviousData gets the agent configuration.
func (c *Client) GetPreviousData(ctx context.Context, startup bool) (map[string][]DataPointGroup, error) {
	endpoint, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "data"))
	if err != nil {
		return nil, fmt.Errorf("creating metrics previous data url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Accept", "avro/binary;v1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting metrics previous data: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("getting metrics previous data got %d: %s", resp.StatusCode, string(body))
	}

	data := map[string][]DataPointGroup{}
	if err = avro.Unmarshal(c.metricsSchema, body, &data); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w: %s", err, string(body))
	}

	return data, nil
}

// Send sends metrics to the metrics service.
func (c *Client) Send(ctx context.Context, data map[string][]DataPointGroup) error {
	endpoint, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "metrics"))
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
