package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/klog/v2"
)

// Setup is configuring the logger.
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

	logLevel, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		logLevel = zerolog.InfoLevel

		log.Debug().Err(err).Str("level", level).Msg("Unspecified or invalid log level, setting the level to default (INFO)...")
	}

	zerolog.SetGlobalLevel(logLevel)

	log.Trace().Str("level", logLevel.String()).Msg("Log level set")

	klog.SetLogger(logrWrapper{logger: log.Logger.With().Str("component", "kubernetes").Logger()})
}
