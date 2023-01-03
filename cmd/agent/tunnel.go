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

package main

import (
	"fmt"
	"net"

	"github.com/ettle/strcase"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/traefik/hub-agent-kubernetes/pkg/tunnel"
	"github.com/urfave/cli/v2"
)

type tunnelCmd struct {
	flags []cli.Flag
}

const (
	flagTraefikTunnelHost = "traefik.tunnel-host"
	flagTraefikTunnelPort = "traefik.tunnel-port"
)

func newTunnelCmd() tunnelCmd {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    flagPlatformURL,
			Usage:   "The URL at which to reach the Hub platform API",
			Value:   "https://platform.hub.traefik.io/agent",
			EnvVars: []string{strcase.ToSNAKE(flagPlatformURL)},
			Hidden:  true,
		},
		&cli.StringFlag{
			Name:     flagToken,
			Usage:    "The token to use for Hub platform API calls",
			EnvVars:  []string{strcase.ToSNAKE(flagToken)},
			Required: true,
		},
		&cli.StringFlag{
			Name:     flagTraefikTunnelHost,
			Usage:    "The Traefik tunnel host",
			EnvVars:  []string{strcase.ToSNAKE(flagTraefikTunnelHost)},
			Required: true,
		},
		&cli.StringFlag{
			Name:     flagTraefikTunnelPort,
			Usage:    "The Traefik tunnel port",
			EnvVars:  []string{strcase.ToSNAKE(flagTraefikTunnelPort)},
			Value:    "9901",
			Required: false,
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
	logger.Setup(cliCtx.String(flagLogLevel), cliCtx.String(flagLogFormat))

	ctx := cliCtx.Context

	platformURL := cliCtx.String(flagPlatformURL)
	token := cliCtx.String(flagToken)

	tunnelClient, err := tunnel.NewClient(platformURL, token)
	if err != nil {
		return fmt.Errorf("create tunnel client: %w", err)
	}

	traefikAddr := net.JoinHostPort(cliCtx.String(flagTraefikTunnelHost), cliCtx.String(flagTraefikTunnelPort))
	tunnelManager := tunnel.NewManager(tunnelClient, traefikAddr, token)
	tunnelManager.Run(ctx)

	return nil
}
