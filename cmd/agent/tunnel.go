package main

import (
	"fmt"

	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/tunnel"
	"github.com/urfave/cli/v2"
)

type tunnelCmd struct {
	flags []cli.Flag
}

func newTunnelCmd() tunnelCmd {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    "platform-url",
			Usage:   "The URL at which to reach the Hub platform API",
			Value:   "https://platform.hub.traefik.io/agent",
			EnvVars: []string{"PLATFORM_URL"},
			Hidden:  true,
		},
		&cli.StringFlag{
			Name:     "token",
			Usage:    "The token to use for Hub platform API calls",
			EnvVars:  []string{"TOKEN"},
			Required: true,
		},
	}

	flags = append(flags, globalFlags()...)

	return tunnelCmd{
		flags: flags,
	}
}

func (c tunnelCmd) build() *cli.Command {
	return &cli.Command{
		Name:   "tunnel",
		Usage:  "Runs the Hub agent tunnel",
		Flags:  c.flags,
		Action: c.run,
	}
}

func (c tunnelCmd) run(cliCtx *cli.Context) error {
	logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

	ctx := cliCtx.Context

	platformURL := cliCtx.String("platform-url")
	token := cliCtx.String("token")

	tunnelClient, err := tunnel.NewClient(platformURL, token)
	if err != nil {
		return fmt.Errorf("create tunnel client: %w", err)
	}

	tunnelManager := tunnel.NewManager(tunnelClient, token)
	tunnelManager.Run(ctx)

	return nil
}
