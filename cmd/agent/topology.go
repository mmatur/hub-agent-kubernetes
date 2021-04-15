package main

import (
	"context"

	"github.com/traefik/neo-agent/pkg/topology"
	"github.com/traefik/neo-agent/pkg/topology/state"
	"github.com/traefik/neo-agent/pkg/topology/store"
	"github.com/urfave/cli/v2"
)

func newTopologyWatcher(ctx context.Context, cliCtx *cli.Context) (*topology.Watcher, error) {
	k8s, err := state.NewFetcher(ctx)
	if err != nil {
		return nil, err
	}

	s, err := store.New(ctx, cliCtx.String("token"), cliCtx.String("platform-url"))
	if err != nil {
		return nil, err
	}

	return topology.NewWatcher(k8s, s), nil
}
