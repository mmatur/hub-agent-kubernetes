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
