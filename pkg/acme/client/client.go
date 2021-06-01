package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PendingError represents an error returned if the requested certificate is not yet available.
type PendingError struct{}

func (a PendingError) Error() string {
	return "certificate issuance pending"
}

// APIError represents an error returned by the API.
type APIError struct {
	StatusCode int
	Message    string `json:"error"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("failed with code %d: %s", a.StatusCode, a.Message)
}

// Certificate represents the certificate returned by the platform.
type Certificate struct {
	Certificate []byte    `json:"certificate"`
	PrivateKey  []byte    `json:"privateKey"`
	Domains     []string  `json:"domains"`
	NotAfter    time.Time `json:"notAfter"`
	NotBefore   time.Time `json:"notBefore"`
}

// Client allows to interact with the certificates service.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a new certificates for the certificates service.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Obtain obtains a certificate for the given domains.
func (c *Client) Obtain(ctx context.Context, domains []string) (Certificate, error) {
	baseURL, err := url.Parse(c.baseURL + "/certificates")
	if err != nil {
		return Certificate{}, fmt.Errorf("parse endpoint: %w", err)
	}

	query := baseURL.Query()
	query.Set("domains", strings.Join(domains, ","))
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return Certificate{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Certificate{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// The certificate is not yet available.
	if resp.StatusCode == http.StatusAccepted {
		return Certificate{}, PendingError{}
	}

	if resp.StatusCode != http.StatusOK {
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return Certificate{}, fmt.Errorf("failed with code %d: decode response: %w", resp.StatusCode, err)
		}

		return Certificate{}, apiErr
	}

	var cert Certificate
	if err := json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		return Certificate{}, fmt.Errorf("decode obtain resp: %w", err)
	}

	return cert, nil
}
