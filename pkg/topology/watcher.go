package topology

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/topology/state"
	"github.com/traefik/hub-agent/pkg/topology/store"
)

// ListenerFunc is a function called by the watcher with the
// current state.
type ListenerFunc func(ctx context.Context, state *state.Cluster)

// Watcher is a process from the Hub agent that watches the topology for changes and
// stores them over time to make them accessible from the SaaS.
type Watcher struct {
	k8s   *state.Fetcher
	store *store.Store

	listenersMu sync.Mutex
	listeners   []ListenerFunc
}

// NewWatcher instantiates a new watcher that uses a fetcher to periodically get the K8S state and a store to write it.
func NewWatcher(f *state.Fetcher, s *store.Store) *Watcher {
	return &Watcher{
		k8s:   f,
		store: s,
	}
}

// AddListener adds a state listener.
func (w *Watcher) AddListener(listener ListenerFunc) {
	w.listenersMu.Lock()
	defer w.listenersMu.Unlock()

	w.listeners = append(w.listeners, listener)
}

// Start runs the watcher process.
func (w *Watcher) Start(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping topology watcher")
			return
		case <-tick.C:
			s, err := w.k8s.FetchState()
			if err != nil {
				log.Error().Err(err).Msg("create state")
				continue
			}

			w.listenersMu.Lock()
			for _, l := range w.listeners {
				l(ctx, s)
			}
			w.listenersMu.Unlock()

			if err = w.store.Write(ctx, s); err != nil {
				log.Error().Err(err).Msg("commit cluster state changes")
			}
		}
	}
}
