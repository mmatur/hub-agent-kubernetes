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

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/apikey"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/oidc"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// NOTE: if we use the same watcher for all resources, then we need to restart it when new CRDs are
// created/removed like for example when Traefik is installed and IngressRoutes are added.
// Always listening to non-existing resources would cause errors.
// Also, if multiple clients of this watcher are not interested in the same resources
// add a parameter to NewWatcher to subscribe only to a subset of events.

type oidcSecret struct {
	ClientSecret string
}

// Watcher watches access control policy resources and builds configurations out of them.
type Watcher struct {
	key string

	configsMu sync.RWMutex
	configs   map[string]*acp.Config
	previous  uint64

	secrets map[string]oidcSecret

	refresh chan struct{}

	switcher *HTTPHandlerSwitcher
}

// NewWatcher returns a new watcher to track ACP resources. It calls the given Updater when an ACP is modified at most
// once every throttle.
func NewWatcher(switcher *HTTPHandlerSwitcher, key string) *Watcher {
	return &Watcher{
		key:      key,
		configs:  make(map[string]*acp.Config),
		secrets:  make(map[string]oidcSecret),
		refresh:  make(chan struct{}, 1),
		switcher: switcher,
	}
}

// Run launches listener if the watcher is dirty.
func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-w.refresh:
			w.configsMu.RLock()

			w.populateSecrets()

			hash, err := hashstructure.Hash(w.configs, hashstructure.FormatV2, nil)
			if err != nil {
				log.Error().Err(err).Msg("Unable to hash")
			}

			w.configsMu.RUnlock()

			if err == nil && w.previous == hash {
				continue
			}

			w.previous = hash

			log.Debug().Msg("Refreshing ACP handlers")

			w.switcher.UpdateHandler(w.buildRoutes(ctx))

		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) populateSecrets() {
	for name, config := range w.configs {
		logger := log.With().Str("acp_name", name).Logger()
		cfg := config.OIDC
		if cfg == nil && config.OIDCGoogle != nil {
			cfg = &config.OIDCGoogle.Config
		}

		if cfg == nil {
			continue
		}

		if cfg.Secret == nil {
			logger.Error().Msg("Secret is missing")
			continue
		}

		logger = logger.With().Str("secret_namespace", cfg.Secret.Namespace).
			Str("secret_name", cfg.Secret.Name).Logger()

		secret, ok := w.secrets[cfg.Secret.Namespace+"@"+cfg.Secret.Name]
		if !ok {
			logger.Error().Msg("Secret is missing")
			continue
		}

		if err := populateSecrets(cfg, secret); err != nil {
			logger.Error().Err(err).Msg("error while populating secrets")
		}
	}
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnAdd(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.AccessControlPolicy:
		w.updateConfigFromPolicy(v)

	case *corev1.Secret:
		w.configsMu.Lock()
		w.secrets[v.Namespace+"@"+v.Name] = oidcSecret{
			ClientSecret: string(v.Data["clientSecret"]),
		}
		w.configsMu.Unlock()

	default:
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received add event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

// OnUpdate implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnUpdate(_, newObj interface{}) {
	switch v := newObj.(type) {
	case *hubv1alpha1.AccessControlPolicy:
		w.updateConfigFromPolicy(v)

	case *corev1.Secret:
		w.configsMu.Lock()
		w.secrets[v.Namespace+"@"+v.Name] = oidcSecret{
			ClientSecret: string(v.Data["clientSecret"]),
		}
		w.configsMu.Unlock()

	default:
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", newObj)).
			Msg("Received update event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) updateConfigFromPolicy(policy *hubv1alpha1.AccessControlPolicy) {
	w.configsMu.Lock()
	defer w.configsMu.Unlock()

	w.configs[policy.ObjectMeta.Name] = acp.ConfigFromPolicy(policy)
	if w.configs[policy.ObjectMeta.Name].OIDC != nil {
		w.configs[policy.ObjectMeta.Name].OIDC.Key = w.key
	}
	if w.configs[policy.ObjectMeta.Name].OIDCGoogle != nil {
		w.configs[policy.ObjectMeta.Name].OIDCGoogle.Key = w.key
	}
}

// OnDelete implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnDelete(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.AccessControlPolicy:
		w.configsMu.Lock()
		delete(w.configs, v.ObjectMeta.Name)
		w.configsMu.Unlock()

	case *corev1.Secret:
		w.configsMu.Lock()
		delete(w.secrets, v.Namespace+"@"+v.Name)
		w.configsMu.Unlock()

	default:
		log.Error().
			Str("component", "acp_watcher").
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received delete event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) buildRoutes(ctx context.Context) http.Handler {
	w.configsMu.RLock()
	defer w.configsMu.RUnlock()

	mux := http.NewServeMux()

	for name, cfg := range w.configs {
		path := "/" + name

		logger := log.With().Str("acp_name", name).Str("acp_type", getACPType(cfg)).Logger()

		route, err := buildRoute(ctx, name, cfg)
		if err != nil {
			logger.Error().Err(err).Msg("create ACP handler")
			continue
		}

		logger.Debug().Msg("Registering ACP handler")

		mux.Handle(path, route)
	}

	return mux
}

func buildRoute(ctx context.Context, name string, cfg *acp.Config) (http.Handler, error) {
	switch {
	case cfg.JWT != nil:
		return jwt.NewHandler(cfg.JWT, name)

	case cfg.BasicAuth != nil:
		return basicauth.NewHandler(cfg.BasicAuth, name)

	case cfg.APIKey != nil:
		return apikey.NewHandler(cfg.APIKey, name)

	case cfg.OIDC != nil:
		return oidc.NewHandler(ctx, cfg.OIDC, name)

	case cfg.OIDCGoogle != nil:
		return oidc.NewHandler(ctx, &cfg.OIDCGoogle.Config, name)

	default:
		return nil, fmt.Errorf("unknown handler type for ACP %s", name)
	}
}

func getACPType(cfg *acp.Config) string {
	switch {
	case cfg.JWT != nil:
		return "JWT"

	case cfg.BasicAuth != nil:
		return "Basic Auth"

	case cfg.APIKey != nil:
		return "API Key"

	case cfg.OIDC != nil:
		return "OIDC"

	case cfg.OIDCGoogle != nil:
		return "OIDCGoogle"

	default:
		return "unknown"
	}
}

func populateSecrets(config *oidc.Config, secret oidcSecret) error {
	if secret.ClientSecret == "" {
		return errors.New("clientSecret is missing in secret")
	}

	config.ClientSecret = secret.ClientSecret

	return nil
}
