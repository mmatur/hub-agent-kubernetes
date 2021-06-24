package heartbeat

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const pingInterval = 5 * time.Minute

// Pinger can ping the platform.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Heartbeater sends pings to the platform.
type Heartbeater struct {
	pinger   Pinger
	interval time.Duration
}

// NewHeartbeater creates a new heartbeater using the given Pinger.
func NewHeartbeater(p Pinger) *Heartbeater {
	return &Heartbeater{
		pinger:   p,
		interval: pingInterval,
	}
}

// Run runs the Heartbeater. This is a blocking method.
func (m *Heartbeater) Run(ctx context.Context) {
	t := time.NewTicker(m.interval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			if err := m.pinger.Ping(ctx); err != nil {
				log.Error().Err(err).Msg("Unable to ping platform")
			}

		case <-ctx.Done():
			return
		}
	}
}
