package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
)

// Client for the alerting service.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client

	token string
}

// NewClient creates an alerting service client.
func NewClient(client *http.Client, baseURL, token string) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid alerting client url: %w", err)
	}

	return &Client{
		baseURL:    base,
		httpClient: client,
		token:      token,
	}, nil
}

// GetRules gets the agent configuration.
func (c *Client) GetRules(ctx context.Context) ([]Rule, error) {
	endpoint, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "rules"))
	if err != nil {
		return nil, fmt.Errorf("creating alerting rules url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting alerting rules: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("getting alerting rules got %d: %s", resp.StatusCode, string(body))
	}

	var rules []Rule
	if err = json.Unmarshal(body, &rules); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w: %s", err, string(body))
	}

	return rules, nil
}

type descriptor struct {
	ID      int    `json:"id"`
	RuleID  string `json:"ruleId"`
	Ingress string `json:"ingress"`
	Service string `json:"service,omitempty"`
}

// PreflightAlerts sends alert descriptors to the server and returns which alerts to send.
func (c *Client) PreflightAlerts(ctx context.Context, data []Alert) ([]Alert, error) {
	endpoint, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "preflight"))
	if err != nil {
		return nil, fmt.Errorf("creating alerts url: %w", err)
	}

	descriptors := make([]descriptor, len(data))
	for i, alert := range data {
		descriptors[i] = descriptor{
			ID:      i,
			RuleID:  alert.RuleID,
			Ingress: alert.Ingress,
			Service: alert.Service,
		}
	}

	body, err := json.Marshal(descriptors)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending alerts: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sending alerts got %d: %s", resp.StatusCode, string(body))
	}

	var pos []int
	err = json.NewDecoder(resp.Body).Decode(&pos)
	if err != nil {
		return nil, err
	}

	if len(pos) == 0 {
		return nil, nil
	}

	var allowed []Alert
	for _, i := range pos {
		if i < 0 || i >= len(data) {
			return nil, fmt.Errorf("invalid alert position: %d", i)
		}
		allowed = append(allowed, data[i])
	}

	return allowed, nil
}

// SendAlerts sends alerts to the server.
func (c *Client) SendAlerts(ctx context.Context, data []Alert) error {
	endpoint, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "notify"))
	if err != nil {
		return fmt.Errorf("creating alerts url: %w", err)
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending alerts: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sending alerts got %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
}
