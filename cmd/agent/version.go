package main

import (
	"os"

	"github.com/traefik/hub-agent-kubernetes/pkg/version"
	"github.com/urfave/cli/v2"
)

type versionCmd struct{}

func newVersionCmd() versionCmd {
	return versionCmd{}
}

func (v versionCmd) build() *cli.Command {
	return &cli.Command{
		Name:   "version",
		Usage:  "Shows the Hub Agent version information",
		Action: v.run,
	}
}

func (v versionCmd) run(*cli.Context) error {
	return version.Print(os.Stdout)
}
