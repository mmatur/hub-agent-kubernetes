package kube

import (
	"fmt"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/logger"
	"k8s.io/client-go/rest"
)

// InClusterConfigWithRetrier returns a new in-cluster configuration that will retry requests that result in transient failures.
func InClusterConfigWithRetrier(maxRetries int) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes in-cluster configuration: %w", err)
	}

	// We first need to get the TLS configuration since we
	// are going to bypass Kubernetes' default HTTP client.
	tlsCfg, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("create TLS config: %w", err)
	}

	rc := retryablehttp.NewClient()
	rc.RetryMax = maxRetries
	rc.HTTPClient.Transport = &http.Transport{TLSClientConfig: tlsCfg}
	rc.Logger = logger.NewRetryableHTTPWrapper(log.Logger.With().Str("component", "kubernetes_client").Logger())

	rrt := &retryablehttp.RoundTripper{Client: rc}

	wt := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if wt != nil {
			wt(rt)
		}
		return rrt
	}
	return cfg, nil
}
