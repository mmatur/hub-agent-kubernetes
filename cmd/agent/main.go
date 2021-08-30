package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ettle/strcase"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

const (
	flagLogLevel  = "log-level"
	flagLogFormat = "log-format"
)

func mainWithCode() int {
	app := &cli.App{
		Name:  "Hub agent CLI",
		Usage: "Manages a Traefik Hub agent installation",
		Commands: []*cli.Command{
			newControllerCmd().build(),
			newAuthServerCmd().build(),
			newRefreshConfigCmd().build(),
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err := app.RunContext(ctx, os.Args)
	if err != nil {
		log.Error().Err(err).Msg("Error while executing command")
		return 255
	}

	return 0
}

func main() {
	os.Exit(mainWithCode())
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
