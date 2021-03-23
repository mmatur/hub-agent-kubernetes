package logger

import (
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Setup is configuring the logger.
func Setup(level, format string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	var w io.Writer
	switch format {
	case "json":
		w = os.Stderr
	case "console":
		w = zerolog.ConsoleWriter{Out: os.Stderr}
	default:
		w = os.Stderr
	}
	log.Logger = zerolog.New(w).With().Timestamp().Caller().Logger()

	logLevel, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		logLevel = zerolog.InfoLevel

		log.Debug().Err(err).Str("level", level).Msg("Unspecified or invalid log level, setting the level to default (INFO)...")
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Trace().Str("level", logLevel.String()).Msg("Log level set")
}
