package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type config struct {
	GitRepo string `json:"gitRepo"`
}

func fetchConfig(ctx context.Context, token, topologyServiceURL string) (*config, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, topologyServiceURL+"/config", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	// TODO Remove this Neo-Path header when token-service is release
	req.Header.Set("Neo-Path", "/topology")

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch git repository config: %s", res.Status)
	}

	var cfg config
	if err := json.NewDecoder(res.Body).Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
