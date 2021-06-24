package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
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
			Name:    "log-level",
			Usage:   "Log level to use (debug, info, warn, error or fatal)",
			EnvVars: []string{"LOG_LEVEL"},
			Value:   "info",
		},
		&cli.StringFlag{
			Name:    "log-format",
			Usage:   "Log format to use (json or console)",
			EnvVars: []string{"LOG_FORMAT"},
			Value:   "json",
			Hidden:  true,
		},
	}
}
