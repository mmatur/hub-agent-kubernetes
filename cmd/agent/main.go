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
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ettle/strcase"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/version"
	"github.com/urfave/cli/v2"
)

const (
	flagLogLevel   = "log-level"
	flagLogFormat  = "log-format"
	flagListenAddr = "listen-addr"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func run() error {
	app := &cli.App{
		Name:    "Traefik Hub agent for Kubernetes",
		Usage:   "Manages a Traefik Hub agent installation",
		Version: version.String(),
		Commands: []*cli.Command{
			newControllerCmd().build(),
			newAuthServerCmd().build(),
			newRefreshConfigCmd().build(),
			newTunnelCmd().build(),
			newVersionCmd().build(),
			newDevPortalCmd().build(),
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return app.RunContext(ctx, os.Args)
}

func globalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    flagLogLevel,
			Usage:   "Log level to use (debug, info, warn, error or fatal)",
			EnvVars: []string{strcase.ToSNAKE(flagLogLevel)},
			Value:   "info",
		},
		&cli.StringFlag{
			Name:    flagLogFormat,
			Usage:   "Log format to use (json or console)",
			EnvVars: []string{strcase.ToSNAKE(flagLogFormat)},
			Value:   "json",
			Hidden:  true,
		},
	}
}
