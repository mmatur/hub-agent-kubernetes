/*
Copyright (C) 2022 Traefik Labs

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

package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup configures the logger.
func Setup(level, format string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	logLevel := zerolog.InfoLevel
	if level != "" {
		var err error
		logLevel, err = zerolog.ParseLevel(strings.ToLower(level))
		if err != nil {
			log.Error().Err(err).Str("LOG_LEVEL", level).Msg("Unspecified or invalid log level, setting the level to default (INFO)...")

			logLevel = zerolog.InfoLevel
		}
	}

	var w io.Writer
	switch format {
	case "json":
		w = os.Stderr
	case "console":
		w = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		}
	default:
		w = os.Stderr
	}

	logCtx := zerolog.New(w).With().Timestamp()
	if logLevel <= zerolog.DebugLevel {
		logCtx = logCtx.Caller()
	}

	log.Logger = logCtx.Logger()
	zerolog.DefaultContextLogger = &log.Logger

	zerolog.SetGlobalLevel(logLevel)

	log.Trace().Str("level", logLevel.String()).Msg("Log level set")
}
