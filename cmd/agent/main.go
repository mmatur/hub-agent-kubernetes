package main

import (
	"context"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/logger"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	app := &cli.App{
		Name:  "Neo-AGENT CLI",
		Usage: "Run neo-agent service",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "log-level",
				Usage:    "The log level for the agent",
				EnvVars:  []string{"LOG_LEVEL"},
				Value:    "info",
				Required: false,
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
		},
		Action: func(cliCtx *cli.Context) error {
			logger.Setup(cliCtx.String("log-level"))

			group, ctx := errgroup.WithContext(context.Background())

			group.Go(func() error {
				return metrics(ctx)
			})

			group.Go(func() error {
				return runTopologyWatcher(ctx, cliCtx)
			})

			group.Go(func() error {
				return acl(ctx)
			})

			return group.Wait()
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func metrics(ctx context.Context) error {
	// TODO
	return nil
}

func acl(ctx context.Context) error {
	// TODO
	return nil
}
