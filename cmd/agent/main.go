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

func main() {
	err := run()
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func run() error {
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
