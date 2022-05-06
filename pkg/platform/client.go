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
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
)

// APIError represents an error returned by the API.
type APIError struct {
	StatusCode int
	Message    string `json:"error"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("failed with code %d: %s", a.StatusCode, a.Message)
}

// Client allows interacting with the cluster service.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new client for the cluster service.
func NewClient(baseURL, token string) *Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = 4
	rc.Logger = logger.NewWrappedLogger(log.Logger.With().Str("component", "platform_client").Logger())

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
	Topology TopologyConfig `json:"topology"`
	Metrics  MetricsConfig  `json:"metrics"`
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

// GetConfig returns the agent configuration.
func (c *Client) GetConfig(ctx context.Context) (Config, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/config", http.NoBody)
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

// GetACPs returns the ACPs related to the agent.
func (c *Client) GetACPs(ctx context.Context) ([]acp.ACP, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/acps", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

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

	var acps []acp.ACP
	if err = json.NewDecoder(resp.Body).Decode(&acps); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return acps, nil
}

// Ping sends a ping to the platform to inform that the agent is alive.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/ping", http.NoBody)
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

// ListVerifiedDomains list verified domains.
func (c *Client) ListVerifiedDomains(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/verified-domains", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", c.baseURL+"/verified-domains", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", c.baseURL+"/verified-domains", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("%q failed with code %d: decode response: %w", c.baseURL+"/verified-domains", resp.StatusCode, err)
		}

		return nil, fmt.Errorf("%q failed with code %d: %s", c.baseURL+"/verified-domains", resp.StatusCode, apiErr.Message)
	}

	var domains []string
	if err = json.NewDecoder(resp.Body).Decode(&domains); err != nil {
		return nil, fmt.Errorf("failed to decode verified domains: %w", err)
	}

	return domains, nil
}

// CreateEdgeIngressReq is the request for creating an edge ingress.
type CreateEdgeIngressReq struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	ServiceName  string `json:"serviceName"`
	ServicePort  int    `json:"servicePort"`
	ACPName      string `json:"acpName"`
	ACPNamespace string `json:"acpNamespace"`
}

// ErrVersionConflict indicates a conflict error on the EdgeIngress resource being modified.
var ErrVersionConflict = errors.New("version conflict")

// CreateEdgeIngress creates an edge ingress.
func (c *Client) CreateEdgeIngress(ctx context.Context, createReq *CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal edge ingress request: %w", err)
	}

	endpoint := c.baseURL + "/edge-ingresses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusConflict:
		return nil, ErrVersionConflict
	case http.StatusCreated:
		var edgeIng edgeingress.EdgeIngress

		if err = json.NewDecoder(resp.Body).Decode(&edgeIng); err != nil {
			return nil, fmt.Errorf("failed to decode edge ingress: %w", err)
		}
		return &edgeIng, nil
	default:
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("%q failed with code %d: decode response: %w", endpoint, resp.StatusCode, err)
		}

		return nil, fmt.Errorf("%q failed with code %d: %s", endpoint, resp.StatusCode, apiErr.Message)
	}
}

// UpdateEdgeIngressReq is a request for updating an edge ingress.
type UpdateEdgeIngressReq struct {
	ServiceName  string `json:"serviceName"`
	ServicePort  int    `json:"servicePort"`
	ACPName      string `json:"acpName"`
	ACPNamespace string `json:"acpNamespace"`
}

// UpdateEdgeIngress updated an edge ingress.
func (c *Client) UpdateEdgeIngress(ctx context.Context, namespace, name, lastKnownVersion string, updateReq *UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal edge ingress request: %w", err)
	}

	id := name + "@" + namespace
	endpoint := c.baseURL + "/edge-ingresses/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Last-Known-Version", lastKnownVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusConflict:
		return nil, ErrVersionConflict
	case http.StatusOK:
		var edgeIng edgeingress.EdgeIngress

		if err = json.NewDecoder(resp.Body).Decode(&edgeIng); err != nil {
			return nil, fmt.Errorf("failed to decode edge ingress: %w", err)
		}
		return &edgeIng, nil
	default:
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("%q failed with code %d: decode response: %w", endpoint, resp.StatusCode, err)
		}

		return nil, fmt.Errorf("%q failed with code %d: %s", endpoint, resp.StatusCode, apiErr.Message)
	}
}

// DeleteEdgeIngress deletes an edge ingress.
func (c *Client) DeleteEdgeIngress(ctx context.Context, lastKnownVersion, namespace, name string) error {
	id := name + "@" + namespace
	endpoint := c.baseURL + "/edge-ingresses/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request for %q: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Last-Known-Version", lastKnownVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %q: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusConflict:
		return ErrVersionConflict
	case http.StatusNoContent:
		return nil
	default:
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("%q failed with code %d: decode response: %w", endpoint, resp.StatusCode, err)
		}

		return fmt.Errorf("%q failed with code %d: %s", endpoint, resp.StatusCode, apiErr.Message)
	}
}

// CreateACP creates an AccessControlPolicy.
func (c *Client) CreateACP(ctx context.Context, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
	acpReq := acp.ACP{
		Name:      policy.Name,
		Namespace: policy.Namespace,
		Config:    *acp.ConfigFromPolicy(policy),
	}
	body, err := json.Marshal(acpReq)
	if err != nil {
		return nil, fmt.Errorf("marshal ACP request: %w", err)
	}

	endpoint := c.baseURL + "/acps"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusConflict:
		return nil, ErrVersionConflict
	case http.StatusCreated:
		var a acp.ACP
		if err = json.NewDecoder(resp.Body).Decode(&a); err != nil {
			return nil, fmt.Errorf("failed to decode ACP: %w", err)
		}

		return &a, nil
	default:
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("%q failed with code %d: decode response: %w", endpoint, resp.StatusCode, err)
		}

		return nil, fmt.Errorf("%q failed with code %d: %s", endpoint, resp.StatusCode, apiErr.Message)
	}
}

// UpdateACP updates an AccessControlPolicy.
func (c *Client) UpdateACP(ctx context.Context, oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
	acpReq := acp.ACP{
		Name:      policy.Name,
		Namespace: policy.Namespace,
		Config:    *acp.ConfigFromPolicy(policy),
	}
	body, err := json.Marshal(acpReq)
	if err != nil {
		return nil, fmt.Errorf("marshal ACP request: %w", err)
	}

	id := policy.Name + "@" + policy.Namespace
	endpoint := c.baseURL + "/acps/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Last-Known-Version", oldVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusConflict:
		return nil, ErrVersionConflict
	case http.StatusOK:
		var a acp.ACP
		if err = json.NewDecoder(resp.Body).Decode(&a); err != nil {
			return nil, fmt.Errorf("failed to decode ACP: %w", err)
		}

		return &a, nil
	default:
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("%q failed with code %d: decode response: %w", endpoint, resp.StatusCode, err)
		}

		return nil, fmt.Errorf("%q failed with code %d: %s", endpoint, resp.StatusCode, apiErr.Message)
	}
}

// DeleteACP deletes an AccessControlPolicy.
func (c *Client) DeleteACP(ctx context.Context, oldVersion, name, namespace string) error {
	id := name + "@" + namespace
	endpoint := c.baseURL + "/acps/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request for %q: %w", endpoint, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Last-Known-Version", oldVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %q: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusConflict:
		return ErrVersionConflict
	case http.StatusNoContent:
		return nil
	default:
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("%q failed with code %d: decode response: %w", endpoint, resp.StatusCode, err)
		}

		return fmt.Errorf("%q failed with code %d: %s", endpoint, resp.StatusCode, apiErr.Message)
	}
}

// GetEdgeIngresses returns the EdgeIngresses related to the agent.
func (c *Client) GetEdgeIngresses(ctx context.Context) ([]edgeingress.EdgeIngress, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/edge-ingresses", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

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

	var edgeIngresses []edgeingress.EdgeIngress
	if err = json.NewDecoder(resp.Body).Decode(&edgeIngresses); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return edgeIngresses, nil
}
