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

package platform

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
	"github.com/traefik/hub-agent-kubernetes/pkg/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIError represents an error returned by the API.
type APIError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"error"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("failed with code %d: %s", a.StatusCode, a.Message)
}

// CreateEdgeIngressReq is the request for creating an edge ingress.
type CreateEdgeIngressReq struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	Service       Service  `json:"service"`
	ACP           *ACP     `json:"acp,omitempty"`
	CustomDomains []string `json:"customDomains,omitempty"`
}

// Service defines the service being exposed by the edge ingress.
type Service struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

// ACP defines the ACP attached to the edge ingress.
type ACP struct {
	Name string `json:"name"`
}

// Config holds the configuration of the offer.
type Config struct {
	Metrics  MetricsConfig `json:"metrics"`
	Features []string      `json:"features"`
}

// MetricsConfig holds the metrics part of the offer config.
type MetricsConfig struct {
	Interval time.Duration `json:"interval"`
	Tables   []string      `json:"tables"`
}

// UpdateEdgeIngressReq is a request for updating an edge ingress.
type UpdateEdgeIngressReq struct {
	Service       Service  `json:"service"`
	ACP           *ACP     `json:"acp,omitempty"`
	CustomDomains []string `json:"customDomains,omitempty"`
}

// CreatePortalReq is the request for creating a portal.
type CreatePortalReq struct {
	Name          string   `json:"name"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Gateway       string   `json:"gateway"`
	CustomDomains []string `json:"customDomains"`
}

// UpdatePortalReq is a request for updating a portal.
type UpdatePortalReq struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Gateway       string   `json:"gateway"`
	HubDomain     string   `json:"hubDomain"`
	CustomDomains []string `json:"customDomains"`
}

// CreateGatewayReq is the request for creating a gateway.
type CreateGatewayReq struct {
	Name          string            `json:"name"`
	Labels        map[string]string `json:"labels"`
	Accesses      []string          `json:"accesses"`
	CustomDomains []string          `json:"customDomains"`
}

// UpdateGatewayReq is a request for updating a gateway.
type UpdateGatewayReq struct {
	Labels        map[string]string `json:"labels"`
	Accesses      []string          `json:"accesses"`
	CustomDomains []string          `json:"customDomains"`
}

// CreateAPIReq is the request for creating an API.
type CreateAPIReq struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	Labels map[string]string `json:"labels,omitempty"`

	PathPrefix string     `json:"pathPrefix"`
	Service    APIService `json:"service"`
}

// UpdateAPIReq is a request for updating an API.
type UpdateAPIReq struct {
	Labels map[string]string `json:"labels,omitempty"`

	PathPrefix string     `json:"pathPrefix"`
	Service    APIService `json:"service"`
}

// APIService is a service used in API struct.
type APIService struct {
	Name        string      `json:"name"`
	Port        int         `json:"port"`
	OpenAPISpec OpenAPISpec `json:"openApiSpec"`
}

// OpenAPISpec is an OpenAPISpec. It can either be fetched from a URL, or Path/Port from the service.
type OpenAPISpec struct {
	URL string `json:"url,omitempty"`

	Path string `json:"path,omitempty"`
	Port int    `json:"port,omitempty"`
}

// CreateCollectionReq is the request for creating a collection.
type CreateCollectionReq struct {
	Name        string               `json:"name"`
	Labels      map[string]string    `json:"labels,omitempty"`
	PathPrefix  string               `json:"pathPrefix,omitempty"`
	APISelector metav1.LabelSelector `json:"apiSelector,omitempty"`
}

// UpdateCollectionReq is a request for updating a collection.
type UpdateCollectionReq struct {
	Labels      map[string]string    `json:"labels,omitempty"`
	PathPrefix  string               `json:"pathPrefix,omitempty"`
	APISelector metav1.LabelSelector `json:"apiSelector,omitempty"`
}

// CreateAccessReq is the request for creating an API access.
type CreateAccessReq struct {
	Name string `json:"name"`

	Labels map[string]string `json:"labels,omitempty"`

	Groups                []string              `json:"groups"`
	APISelector           *metav1.LabelSelector `json:"apiSelector,omitempty"`
	APICollectionSelector *metav1.LabelSelector `json:"apiCollectionSelector,omitempty"`
}

// UpdateAccessReq is a request for updating an API access.
type UpdateAccessReq struct {
	Labels map[string]string `json:"labels,omitempty"`

	Groups                []string              `json:"groups"`
	APISelector           *metav1.LabelSelector `json:"apiSelector,omitempty"`
	APICollectionSelector *metav1.LabelSelector `json:"apiCollectionSelector,omitempty"`
}

// Command defines patch operation to apply on the cluster.
type Command struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"createdAt"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
}

// CommandExecutionStatus describes the execution status of a command.
type CommandExecutionStatus string

// The different CommandExecutionStatus available.
const (
	CommandExecutionStatusSuccess CommandExecutionStatus = "success"
	CommandExecutionStatusFailure CommandExecutionStatus = "failure"
)

// CommandExecutionReportError holds details about an execution failure.
type CommandExecutionReportError struct {
	// Type identifies the reason of the error.
	Type string `json:"type"`

	// Data is a freeform Type dependent value.
	Data interface{} `json:"data,omitempty"`
}

// CommandExecutionReport describes the output of a command execution.
type CommandExecutionReport struct {
	ID     string                       `json:"id"`
	Status CommandExecutionStatus       `json:"status"`
	Error  *CommandExecutionReportError `json:"error,omitempty"`
}

// NewErrorCommandExecutionReport creates a new CommandExecutionReport with a status CommandExecutionStatusFailure.
func NewErrorCommandExecutionReport(id string, err CommandExecutionReportError) *CommandExecutionReport {
	return &CommandExecutionReport{
		ID:     id,
		Status: CommandExecutionStatusFailure,
		Error:  &err,
	}
}

// NewSuccessCommandExecutionReport creates a new CommandExecutionReport with a status CommandExecutionStatusSuccess.
func NewSuccessCommandExecutionReport(id string) *CommandExecutionReport {
	return &CommandExecutionReport{
		ID:     id,
		Status: CommandExecutionStatusSuccess,
	}
}

type linkClusterReq struct {
	KubeID   string `json:"kubeId"`
	Platform string `json:"platform"`
	Version  string `json:"version"`
}

type linkClusterResp struct {
	ClusterID string `json:"clusterId"`
}

type fetchResp struct {
	Version  int64         `json:"version"`
	Topology state.Cluster `json:"topology"`
}

type patchResp struct {
	Version int64 `json:"version"`
}

// Client allows interacting with the cluster service.
type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

// NewClient creates a new client for the cluster service.
func NewClient(baseURL, token string) (*Client, error) {
	u, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse client url: %w", err)
	}

	client := retryablehttp.NewClient()
	client.RetryMax = 4
	client.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "platform_client").Logger())

	return &Client{
		baseURL:    u,
		token:      token,
		httpClient: client.StandardClient(),
	}, nil
}

// Link links the agent to the given Kubernetes ID.
func (c *Client) Link(ctx context.Context, kubeID string) (string, error) {
	body, err := json.Marshal(linkClusterReq{KubeID: kubeID, Platform: "kubernetes", Version: version.Version()})
	if err != nil {
		return "", fmt.Errorf("marshal link agent request: %w", err)
	}

	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "link"))
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

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

// GetConfig returns the agent configuration.
func (c *Client) GetConfig(ctx context.Context) (Config, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "config"))
	if err != nil {
		return Config{}, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return Config{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Config{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
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
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "ping"))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}

	return nil
}

// SetVersionStatus sends the current version status to the platform.
func (c *Client) SetVersionStatus(ctx context.Context, status version.Status) error {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "version-status"))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	body, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}

	return nil
}

// ListVerifiedDomains list verified domains.
func (c *Client) ListVerifiedDomains(ctx context.Context) ([]string, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "verified-domains"))
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request for %q: %w", baseURL.String(), err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", baseURL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return nil, apiErr
	}

	var domains []string
	if err = json.NewDecoder(resp.Body).Decode(&domains); err != nil {
		return nil, fmt.Errorf("failed to decode verified domains: %w", err)
	}

	return domains, nil
}

// CreateEdgeIngress creates an edge ingress.
func (c *Client) CreateEdgeIngress(ctx context.Context, createReq *CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal edge ingress request: %w", err)
	}

	var e edgeingress.EdgeIngress
	if err = c.createResource(ctx, "edge-ingresses", body, &e); err != nil {
		return nil, fmt.Errorf("create edge ingress: %w", err)
	}

	return &e, nil
}

// GetEdgeIngresses returns the EdgeIngresses related to the agent.
func (c *Client) GetEdgeIngresses(ctx context.Context) ([]edgeingress.EdgeIngress, error) {
	var edgeIngresses []edgeingress.EdgeIngress
	if err := c.listResource(ctx, "edge-ingresses", &edgeIngresses); err != nil {
		return nil, fmt.Errorf("list edge ingresses: %w", err)
	}

	return edgeIngresses, nil
}

// UpdateEdgeIngress updated an edge ingress.
func (c *Client) UpdateEdgeIngress(ctx context.Context, namespace, name, lastKnownVersion string, updateReq *UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal edge ingress request: %w", err)
	}

	var e edgeingress.EdgeIngress
	if err = c.updateResource(ctx, "edge-ingresses", name+"@"+namespace, lastKnownVersion, body, &e); err != nil {
		return nil, fmt.Errorf("update edge ingress: %w", err)
	}

	return &e, nil
}

// DeleteEdgeIngress deletes an edge ingress.
func (c *Client) DeleteEdgeIngress(ctx context.Context, namespace, name, lastKnownVersion string) error {
	if err := c.deleteResource(ctx, "edge-ingresses", name+"@"+namespace, lastKnownVersion); err != nil {
		return fmt.Errorf("delete edge ingress: %w", err)
	}

	return nil
}

// CreateACP creates an AccessControlPolicy.
func (c *Client) CreateACP(ctx context.Context, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
	acpReq := acp.ACP{
		Name:   policy.Name,
		Config: *acp.ConfigFromPolicy(policy),
	}
	body, err := json.Marshal(acpReq)
	if err != nil {
		return nil, fmt.Errorf("marshal ACP request: %w", err)
	}

	var a acp.ACP
	if err = c.createResource(ctx, "acps", body, &a); err != nil {
		return nil, fmt.Errorf("create ACP: %w", err)
	}

	return &a, nil
}

// GetACPs returns the ACPs related to the agent.
func (c *Client) GetACPs(ctx context.Context) ([]acp.ACP, error) {
	var acps []acp.ACP
	if err := c.listResource(ctx, "acps", &acps); err != nil {
		return nil, fmt.Errorf("list acps: %w", err)
	}

	return acps, nil
}

// UpdateACP updates an AccessControlPolicy.
func (c *Client) UpdateACP(ctx context.Context, oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
	acpReq := acp.ACP{
		Name:   policy.Name,
		Config: *acp.ConfigFromPolicy(policy),
	}
	body, err := json.Marshal(acpReq)
	if err != nil {
		return nil, fmt.Errorf("marshal ACP request: %w", err)
	}

	var a acp.ACP
	if err = c.updateResource(ctx, "acps", policy.Name, oldVersion, body, &a); err != nil {
		return nil, fmt.Errorf("update ACP: %w", err)
	}

	return &a, nil
}

// DeleteACP deletes an AccessControlPolicy.
func (c *Client) DeleteACP(ctx context.Context, oldVersion, name string) error {
	if err := c.deleteResource(ctx, "acps", name, oldVersion); err != nil {
		return fmt.Errorf("delete acp: %w", err)
	}

	return nil
}

// CreatePortal creates a portal.
func (c *Client) CreatePortal(ctx context.Context, createReq *CreatePortalReq) (*api.Portal, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal portal request: %w", err)
	}

	var p api.Portal
	if err = c.createResource(ctx, "portals", body, &p); err != nil {
		return nil, fmt.Errorf("create portal: %w", err)
	}

	return &p, nil
}

// GetPortals fetches the portals available for this agent.
func (c *Client) GetPortals(ctx context.Context) ([]api.Portal, error) {
	var portals []api.Portal
	if err := c.listResource(ctx, "portals", &portals); err != nil {
		return nil, fmt.Errorf("list portals: %w", err)
	}

	return portals, nil
}

// UpdatePortal updates a portal.
func (c *Client) UpdatePortal(ctx context.Context, name, lastKnownVersion string, updateReq *UpdatePortalReq) (*api.Portal, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal portal request: %w", err)
	}

	var p api.Portal
	if err = c.updateResource(ctx, "portals", name, lastKnownVersion, body, &p); err != nil {
		return nil, fmt.Errorf("update portal: %w", err)
	}

	return &p, nil
}

// DeletePortal deletes a portal.
func (c *Client) DeletePortal(ctx context.Context, name, lastKnownVersion string) error {
	if err := c.deleteResource(ctx, "portals", name, lastKnownVersion); err != nil {
		return fmt.Errorf("delete portal: %w", err)
	}

	return nil
}

// CreateGateway creates a gateway.
func (c *Client) CreateGateway(ctx context.Context, createReq *CreateGatewayReq) (*api.Gateway, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway request: %w", err)
	}

	var g api.Gateway
	if err = c.createResource(ctx, "gateways", body, &g); err != nil {
		return nil, fmt.Errorf("create gateway: %w", err)
	}

	return &g, nil
}

// GetGateways fetches the gateways available for this agent.
func (c *Client) GetGateways(ctx context.Context) ([]api.Gateway, error) {
	var gateways []api.Gateway
	if err := c.listResource(ctx, "gateways", &gateways); err != nil {
		return nil, fmt.Errorf("list gateways: %w", err)
	}

	return gateways, nil
}

// UpdateGateway updates a gateway.
func (c *Client) UpdateGateway(ctx context.Context, name, lastKnownVersion string, updateReq *UpdateGatewayReq) (*api.Gateway, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway request: %w", err)
	}

	var g api.Gateway
	if err = c.updateResource(ctx, "gateways", name, lastKnownVersion, body, &g); err != nil {
		return nil, fmt.Errorf("update gateway: %w", err)
	}

	return &g, nil
}

// DeleteGateway deletes a gateway.
func (c *Client) DeleteGateway(ctx context.Context, name, lastKnownVersion string) error {
	if err := c.deleteResource(ctx, "gateways", name, lastKnownVersion); err != nil {
		return fmt.Errorf("delete gateway: %w", err)
	}

	return nil
}

// CreateAPI creates an API.
func (c *Client) CreateAPI(ctx context.Context, createReq *CreateAPIReq) (*api.API, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal api request: %w", err)
	}

	var a api.API
	if err = c.createResource(ctx, "apis", body, &a); err != nil {
		return nil, fmt.Errorf("create api: %w", err)
	}

	return &a, nil
}

// CreateAccess creates an API access.
func (c *Client) CreateAccess(ctx context.Context, createReq *CreateAccessReq) (*api.Access, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal access request: %w", err)
	}

	var a api.Access
	if err = c.createResource(ctx, "accesses", body, &a); err != nil {
		return nil, fmt.Errorf("create access: %w", err)
	}

	return &a, nil
}

// GetAPIs fetches the APIs available for this agent.
func (c *Client) GetAPIs(ctx context.Context) ([]api.API, error) {
	var apis []api.API
	if err := c.listResource(ctx, "apis", &apis); err != nil {
		return nil, fmt.Errorf("list apis: %w", err)
	}

	return apis, nil
}

// UpdateAPI updates an API.
func (c *Client) UpdateAPI(ctx context.Context, namespace, name, lastKnownVersion string, updateReq *UpdateAPIReq) (*api.API, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal api request: %w", err)
	}

	var a api.API
	if err = c.updateResource(ctx, "apis", name+"@"+namespace, lastKnownVersion, body, &a); err != nil {
		return nil, fmt.Errorf("update api: %w", err)
	}

	return &a, nil
}

// GetAccesses fetches the accesses available for this agent.
func (c *Client) GetAccesses(ctx context.Context) ([]api.Access, error) {
	var accesses []api.Access
	if err := c.listResource(ctx, "accesses", &accesses); err != nil {
		return nil, fmt.Errorf("list accesses: %w", err)
	}

	return accesses, nil
}

// UpdateAccess updates an API access.
func (c *Client) UpdateAccess(ctx context.Context, name, lastKnownVersion string, updateReq *UpdateAccessReq) (*api.Access, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal access request: %w", err)
	}

	var a api.Access
	if err = c.updateResource(ctx, "accesses", name, lastKnownVersion, body, &a); err != nil {
		return nil, fmt.Errorf("update access: %w", err)
	}

	return &a, nil
}

// DeleteAPI deletes an API.
func (c *Client) DeleteAPI(ctx context.Context, namespace, name, lastKnownVersion string) error {
	if err := c.deleteResource(ctx, "apis", name+"@"+namespace, lastKnownVersion); err != nil {
		return fmt.Errorf("delete api: %w", err)
	}

	return nil
}

// CreateCollection creates a collection.
func (c *Client) CreateCollection(ctx context.Context, createReq *CreateCollectionReq) (*api.Collection, error) {
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("marshal collection request: %w", err)
	}

	var collection api.Collection
	if err = c.createResource(ctx, "collections", body, &collection); err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}

	return &collection, nil
}

// GetCollections fetches the collections available for this agent.
func (c *Client) GetCollections(ctx context.Context) ([]api.Collection, error) {
	var collections []api.Collection
	if err := c.listResource(ctx, "collections", &collections); err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	return collections, nil
}

// UpdateCollection updates a collection.
func (c *Client) UpdateCollection(ctx context.Context, name, lastKnownVersion string, updateReq *UpdateCollectionReq) (*api.Collection, error) {
	body, err := json.Marshal(updateReq)
	if err != nil {
		return nil, fmt.Errorf("marshal collection request: %w", err)
	}

	var collection api.Collection
	if err = c.updateResource(ctx, "collections", name, lastKnownVersion, body, &collection); err != nil {
		return nil, fmt.Errorf("update collection: %w", err)
	}

	return &collection, nil
}

// DeleteCollection deletes a collection.
func (c *Client) DeleteCollection(ctx context.Context, name, lastKnownVersion string) error {
	if err := c.deleteResource(ctx, "collections", name, lastKnownVersion); err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}

	return nil
}

// DeleteAccess deletes an API access.
func (c *Client) DeleteAccess(ctx context.Context, name, lastKnownVersion string) error {
	if err := c.deleteResource(ctx, "accesses", name, lastKnownVersion); err != nil {
		return fmt.Errorf("delete access: %w", err)
	}

	return nil
}

// GetWildcardCertificate gets a certificate for the workspace.
func (c *Client) GetWildcardCertificate(ctx context.Context) (edgeingress.Certificate, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "wildcard-certificate"))
	if err != nil {
		return edgeingress.Certificate{}, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return edgeingress.Certificate{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return edgeingress.Certificate{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return edgeingress.Certificate{}, apiErr
	}

	var cert edgeingress.Certificate
	if err = json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		return edgeingress.Certificate{}, fmt.Errorf("decode get wildcard certificate resp: %w", err)
	}

	return cert, nil
}

// GetCertificateByDomains gets a certificate for the given domains.
func (c *Client) GetCertificateByDomains(ctx context.Context, domains []string) (edgeingress.Certificate, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "certificate"))
	if err != nil {
		return edgeingress.Certificate{}, fmt.Errorf("parse endpoint: %w", err)
	}

	query := baseURL.Query()
	for _, domain := range domains {
		query.Add("domains", domain)
	}
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return edgeingress.Certificate{}, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return edgeingress.Certificate{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return edgeingress.Certificate{}, apiErr
	}

	var cert edgeingress.Certificate
	if err = json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		return edgeingress.Certificate{}, fmt.Errorf("decode get certificate resp: %w", err)
	}

	return cert, nil
}

// FetchTopology fetches the topology.
func (c *Client) FetchTopology(ctx context.Context) (topology state.Cluster, topoVersion int64, err error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "topology"))
	if err != nil {
		return state.Cluster{}, 0, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return state.Cluster{}, 0, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept-Encoding", "gzip")
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return state.Cluster{}, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readBody(resp)
	if err != nil {
		return state.Cluster{}, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(body, &apiErr); err != nil {
			apiErr.Message = string(body)
		}

		return state.Cluster{}, 0, apiErr
	}

	var r fetchResp
	if err = json.Unmarshal(body, &r); err != nil {
		return state.Cluster{}, 0, fmt.Errorf("decode topology: %w", err)
	}

	return r.Topology, r.Version, nil
}

// PatchTopology submits a JSON Merge Patch to the platform containing the difference in the topology since
// its last synchronization. The last known topology version must be provided. This version can be obtained
// by calling the FetchTopology method.
func (c *Client) PatchTopology(ctx context.Context, patch []byte, lastKnownVersion int64) (int64, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "topology"))
	if err != nil {
		return 0, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := newGzippedRequestWithContext(ctx, http.MethodPatch, baseURL.String(), patch)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/merge-patch+json")
	req.Header.Set("Last-Known-Version", strconv.FormatInt(lastKnownVersion, 10))
	version.SetUserAgent(req)

	// This operation cannot be retried without calling FetchTopology in between.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return 0, apiErr
	}

	var body patchResp
	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode topology: %w", err)
	}

	return body.Version, nil
}

// ListPendingCommands fetches the commands to apply on the cluster.
func (c *Client) ListPendingCommands(ctx context.Context) ([]Command, error) {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "commands"))
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
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
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return nil, apiErr
	}

	var commands []Command
	if err = json.NewDecoder(resp.Body).Decode(&commands); err != nil {
		return nil, fmt.Errorf("decode list commands resp: %w", err)
	}

	return commands, nil
}

// SubmitCommandReports submits the given command execution reports.
func (c *Client) SubmitCommandReports(ctx context.Context, reports []CommandExecutionReport) error {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, "command-reports"))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	body, err := json.Marshal(reports)
	if err != nil {
		return fmt.Errorf("marshal command reports: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewReader(body))
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
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}

	return nil
}

func newGzippedRequestWithContext(ctx context.Context, verb, u string, body []byte) (*http.Request, error) {
	var compressedBody bytes.Buffer

	writer := gzip.NewWriter(&compressedBody)
	_, err := writer.Write(body)
	if err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, verb, u, &compressedBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Encoding", "gzip")

	return req, nil
}

func readBody(resp *http.Response) ([]byte, error) {
	contentEncoding := resp.Header.Get("Content-Encoding")

	switch contentEncoding {
	case "gzip":
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer func() { _ = reader.Close() }()

		return io.ReadAll(reader)
	case "":
		return io.ReadAll(resp.Body)
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", contentEncoding)
	}
}

func (c *Client) createResource(ctx context.Context, apiPath string, body []byte, obj any) error {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, apiPath))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request for %q: %w", baseURL.String(), err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %q: %w", baseURL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusCreated:
		if err = json.NewDecoder(resp.Body).Decode(&obj); err != nil {
			return fmt.Errorf("failed to decode resource from %q: %w", baseURL.String(), err)
		}
		return nil
	default:
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}
}

func (c *Client) listResource(ctx context.Context, apiPath string, objs any) error {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, apiPath))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}

	if err = json.NewDecoder(resp.Body).Decode(&objs); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	return nil
}

func (c *Client) deleteResource(ctx context.Context, apiPath, name, lastKnownVersion string) error {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, apiPath, name))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL.String(), http.NoBody)
	if err != nil {
		return fmt.Errorf("build request for %q: %w", baseURL.String(), err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Last-Known-Version", lastKnownVersion)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %q: %w", baseURL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	default:
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}
}

func (c *Client) updateResource(ctx context.Context, apiPath, name, lastKnownVersion string, body []byte, obj any) error {
	baseURL, err := c.baseURL.Parse(path.Join(c.baseURL.Path, apiPath, name))
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request for %q: %w", baseURL.String(), err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Last-Known-Version", lastKnownVersion)
	version.SetUserAgent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %q: %w", baseURL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		if err = json.NewDecoder(resp.Body).Decode(&obj); err != nil {
			return fmt.Errorf("failed to decode resource from %q: %w", baseURL.String(), err)
		}

		return nil
	default:
		all, _ := io.ReadAll(resp.Body)

		apiErr := APIError{StatusCode: resp.StatusCode}
		if err = json.Unmarshal(all, &apiErr); err != nil {
			apiErr.Message = string(all)
		}

		return apiErr
	}
}
