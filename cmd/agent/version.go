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
