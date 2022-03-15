package main

import (
	"context"

	"github.com/traefik/hub-agent-kubernetes/pkg/topology"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/state"
	"github.com/traefik/hub-agent-kubernetes/pkg/topology/store"
)

func newTopologyWatcher(ctx context.Context, fetcher *state.Fetcher, storeCfg store.Config) (*topology.Watcher, error) {
	s, err := store.New(ctx, storeCfg)
	if err != nil {
		return nil, err
	}

	return topology.NewWatcher(fetcher, s), nil
}
