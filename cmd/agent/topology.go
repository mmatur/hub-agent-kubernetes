package main

import (
	"context"

	"github.com/traefik/neo-agent/pkg/topology"
	"github.com/traefik/neo-agent/pkg/topology/state"
	"github.com/traefik/neo-agent/pkg/topology/store"
	"github.com/urfave/cli/v2"
)

func runTopologyWatcher(ctx context.Context, cliCtx *cli.Context) error {
	k8s, err := state.NewFetcher(ctx)
	if err != nil {
		return err
	}

	s, err := store.New(ctx, cliCtx.String("token"), cliCtx.String("platform-url"))
	if err != nil {
		return err
	}

	topology.NewWatcher(k8s, s).Start(ctx)

	return nil
}
