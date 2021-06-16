package logger

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	"github.com/rs/zerolog"
)

// RetryableHTTPWrapper wraps our logger and implements retryablehttp.LeveledLogger.
// The retry library we're using uses structured logging but sends fields as pairs of keys and values,
// so we need to adapt them to our logger.
type RetryableHTTPWrapper struct {
	logger zerolog.Logger
}

// NewRetryableHTTPWrapper creates an implementation of the retryablehttp.LeveledLogger.
func NewRetryableHTTPWrapper(logger zerolog.Logger) *RetryableHTTPWrapper {
	return &RetryableHTTPWrapper{logger: logger}
}

// Error starts a new message with error level.
func (l RetryableHTTPWrapper) Error(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Error(), msg, keysAndValues...)
}

// Info starts a new message with info level.
func (l RetryableHTTPWrapper) Info(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Info(), msg, keysAndValues...)
}

// Debug starts a new message with debug level.
func (l RetryableHTTPWrapper) Debug(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Debug(), msg, keysAndValues...)
}

// Warn starts a new message with warn level.
func (l RetryableHTTPWrapper) Warn(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Warn(), msg, keysAndValues...)
}

func logWithLevel(ev *zerolog.Event, msg string, kvs ...interface{}) {
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

type logrWrapper struct {
	logger zerolog.Logger
	name   string
}

func (l logrWrapper) Enabled() bool {
	return true
}

func (l logrWrapper) Info(msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Debug(), msg, keysAndValues...)
}

func (l logrWrapper) Error(err error, msg string, keysAndValues ...interface{}) {
	logWithLevel(l.logger.Error(), msg, keysAndValues...)
}

func (l logrWrapper) V(_ int) logr.Logger {
	return l
}

func (l logrWrapper) WithValues(kvs ...interface{}) logr.Logger {
	r := l.logger.With()

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
				r = r.Str(key, s.String())
			} else {
				r = r.Interface(key, val)
			}
		}
	}

	return logrWrapper{logger: r.Logger()}
}

func (l logrWrapper) WithName(name string) logr.Logger {
	return logrWrapper{logger: l.logger.With().Str("logger_name", name+l.name).Logger(), name: name + l.name}
}
