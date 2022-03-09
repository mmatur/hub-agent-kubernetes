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

	log.Logger = zerolog.New(w).With().Timestamp().Logger()
	zerolog.DefaultContextLogger = &log.Logger

	logLevel := zerolog.InfoLevel
	if level != "" {
		var err error
		logLevel, err = zerolog.ParseLevel(strings.ToLower(level))
		if err != nil {
			log.Error().Err(err).Str("LOG_LEVEL", level).Msg("Unspecified or invalid log level, setting the level to default (INFO)...")

			logLevel = zerolog.InfoLevel
		}
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Trace().Str("level", logLevel.String()).Msg("Log level set")
}
