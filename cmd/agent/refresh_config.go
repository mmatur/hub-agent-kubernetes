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
	"os"
	"strconv"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/logger"
	"github.com/urfave/cli/v2"
)

type refreshConfigCmd struct {
	flags []cli.Flag
}

func newRefreshConfigCmd() refreshConfigCmd {
	return refreshConfigCmd{
		flags: globalFlags(),
	}
}

func (c refreshConfigCmd) build() *cli.Command {
	return &cli.Command{
		Name:   "refresh-config",
		Usage:  "Refresh agent configuration",
		Flags:  c.flags,
		Action: c.run,
	}
}

func (c refreshConfigCmd) run(cliCtx *cli.Context) error {
	logger.Setup(cliCtx.String(flagLogLevel), cliCtx.String(flagLogFormat))

	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		log.Error().Err(err).Msg("Unable to read PID file")
		return err
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return err
	}

	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		log.Error().Err(err).Msg("Unable to send SIGHUP to hub-agent-kubernetes")
		return err
	}

	return nil
}
