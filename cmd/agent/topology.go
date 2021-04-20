package main

import (
	"context"

	"github.com/traefik/neo-agent/pkg/topology"
	"github.com/traefik/neo-agent/pkg/topology/state"
	"github.com/traefik/neo-agent/pkg/topology/store"
)

func newTopologyWatcher(ctx context.Context, neoClusterID string, storeCfg store.Config) (*topology.Watcher, error) {
	k8s, err := state.NewFetcher(ctx, neoClusterID)
	if err != nil {
		return nil, err
	}

	s, err := store.New(ctx, storeCfg)
	if err != nil {
		return nil, err
	}

	return topology.NewWatcher(k8s, s), nil
}
