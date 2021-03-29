package kube

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/rs/zerolog"
)

// wrappedLogger wraps our logger and implements retryablehttp.LeveledLogger.
type wrappedLogger struct {
	logger zerolog.Logger
}

func (l wrappedLogger) Error(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Error(), msg, keysAndValues...)
}

func (l wrappedLogger) Info(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Info(), msg, keysAndValues...)
}

func (l wrappedLogger) Debug(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Debug(), msg, keysAndValues...)
}

func (l wrappedLogger) Warn(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Warn(), msg, keysAndValues...)
}

func logWithLevel(ev *zerolog.Event, msg string, kvs ...interface{}) {
	// The retry library we're using uses structured logging but sends
	// fields as pairs of keys and values, so we need to adapt them to
	// our logger.
	if len(kvs)%2 == 0 {
		for i := 0; i < len(kvs)-1; i += 2 {
			// The first item of the pair (the key) is supposed to be a string.
			key, ok := kvs[i].(string)
			if !ok {
				continue
			}
			val := kvs[i+1]

			var s fmt.Stringer
			if s, ok = val.(fmt.Stringer); ok {
				ev.Str(key, s.String())
			} else {
				ev.Interface(key, val)
			}
		}
	}

	// Capitalize first character.
	first := true
	msg = strings.Map(func(r rune) rune {
		if first {
			first = false
			return unicode.ToTitle(r)
		}
		return r
	}, msg)

	ev.Msg(msg)
}
