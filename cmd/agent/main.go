package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/logger"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	app := &cli.App{
		Name:  "Neo agent CLI",
		Usage: "Run neo-agent service",
		Flags: allFlags(),
		Action: func(cliCtx *cli.Context) error {
			logger.Setup(cliCtx.String("log-level"), cliCtx.String("log-format"))

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			group, ctx := errgroup.WithContext(ctx)

			group.Go(func() error {
				return runMetrics(ctx, cliCtx)
			})

			group.Go(func() error {
				return runTopologyWatcher(ctx, cliCtx)
			})

			group.Go(func() error { return accessControl(ctx, cliCtx) })

			group.Go(func() error { return runAuth(ctx, cliCtx) })

			return group.Wait()
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func allFlags() []cli.Flag {
	flgs := []cli.Flag{
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
		},
		&cli.StringFlag{
			Name:     "platform-url",
			Usage:    "The URL at which to reach the Neo platform API",
			EnvVars:  []string{"PLATFORM_URL"},
			Required: true,
		},
		&cli.StringFlag{
			Name:     "token",
			Usage:    "The token to use for Neo platform API calls",
			EnvVars:  []string{"TOKEN"},
			Required: true,
		},
	}

	flgs = append(flgs, metricsFlags()...)
	flgs = append(flgs, authFlags()...)
	flgs = append(flgs, acpFlags()...)

	return flgs
}
