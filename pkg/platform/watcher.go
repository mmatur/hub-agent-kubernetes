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

package platform

import (
	"context"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// ConfigWatcher watches hub agent configuration.
type ConfigWatcher struct {
	client   *Client
	interval time.Duration

	currentCfg Config

	listenersMu sync.RWMutex
	listeners   []func(cfg Config)
}

// NewConfigWatcher return a new ConfigWatcher.
func NewConfigWatcher(interval time.Duration, c *Client) *ConfigWatcher {
	return &ConfigWatcher{
		client:   c,
		interval: interval,
	}
}

// Run runs ConfigWatcher.
func (w *ConfigWatcher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			if err := w.reload(ctx); err != nil {
				log.Error().Err(err).Msg("Unable to reload hub-agent-kubernetes configuration after receiving SIGHUP")
			}
		case <-t.C:
			if err := w.reload(ctx); err != nil {
				log.Error().Err(err).Msg("Unable to reload hub-agent-kubernetes configuration")
			}
		}
	}
}

// AddListener adds a listeners to the ConfigWatcher.
func (w *ConfigWatcher) AddListener(listener func(cfg Config)) {
	w.listenersMu.Lock()
	defer w.listenersMu.Unlock()

	w.listeners = append(w.listeners, listener)
}

func (w *ConfigWatcher) reload(ctx context.Context) error {
	cfg, err := w.client.GetConfig(ctx)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(w.currentCfg, cfg) {
		return nil
	}

	w.currentCfg = cfg
	w.listenersMu.RLock()
	for _, listener := range w.listeners {
		go listener(cfg)
	}
	w.listenersMu.RUnlock()

	return nil
}
