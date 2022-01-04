package main

import (
	"os"
	"strconv"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/logger"
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
	logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

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
		log.Error().Err(err).Msg("Unable to send SIGHUP to hub-agent")
		return err
	}

	return nil
}
