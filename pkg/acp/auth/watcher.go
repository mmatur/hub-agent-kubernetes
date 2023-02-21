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
	"fmt"
	"net/http"
	"sync"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/apikey"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/jwt"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/oauthintro"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/oidc"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubv1alpha1lister "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/listers/hub/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// NOTE: if we use the same watcher for all resources, then we need to restart it when new CRDs are
// created/removed like for example when Traefik is installed and IngressRoutes are added.
// Always listening to non-existing resources would cause errors.
// Also, if multiple clients of this watcher are not interested in the same resources
// add a parameter to NewWatcher to subscribe only to a subset of events.

// Watcher watches access control policy resources and builds configurations out of them.
type Watcher struct {
	configsMu sync.RWMutex
	configs   map[string]*acp.Config
	previous  uint64

	acps               hubv1alpha1lister.AccessControlPolicyLister
	secrets            acp.SecretGetter
	secretRefCounterMu sync.RWMutex
	secretRefCounter   map[string]int

	refresh chan struct{}

	switcher *HTTPHandlerSwitcher
}

// NewWatcher returns a new watcher to track ACP resources. It calls the given Updater when an ACP is modified at most
// once every throttle.
func NewWatcher(switcher *HTTPHandlerSwitcher, acps hubv1alpha1lister.AccessControlPolicyLister, secrets acp.SecretGetter) *Watcher {
	return &Watcher{
		configs:          make(map[string]*acp.Config),
		acps:             acps,
		secrets:          secrets,
		secretRefCounter: make(map[string]int),
		refresh:          make(chan struct{}, 1),
		switcher:         switcher,
	}
}

// Run launches listener if the watcher is dirty.
func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-w.refresh:
			configs, err := w.makeConfigs()
			if err != nil {
				log.Error().Err(err).Msg("Could not build ACP configs")
			}

			hash, err := hashstructure.Hash(configs, hashstructure.FormatV2, nil)
			if err != nil {
				log.Error().Err(err).Msg("Could not to compute ACP configs hash")
			}

			if err == nil && w.previous == hash {
				continue
			}

			w.configsMu.Lock()
			w.configs = configs
			w.configsMu.Unlock()

			w.previous = hash

			log.Debug().Msg("Refreshing ACP handlers")

			w.switcher.UpdateHandler(w.buildRoutes(ctx))

		case <-ctx.Done():
			return
		}
	}
}

// OnAdd implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnAdd(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.AccessControlPolicy:
		refs := secretReferences(v)
		w.secretRefCounterMu.Lock()
		for _, ref := range refs {
			w.secretRefCounter[ref]++
		}
		w.secretRefCounterMu.Unlock()

	case *corev1.Secret:
		w.secretRefCounterMu.RLock()
		c := w.secretRefCounter[secretKey(v.Name, v.Namespace)]
		w.secretRefCounterMu.RUnlock()
		if c == 0 {
			return
		}

	default:
		log.Error().
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
func (w *Watcher) OnUpdate(oldObj, newObj interface{}) {
	switch v := newObj.(type) {
	case *hubv1alpha1.AccessControlPolicy:
		oldRefs := secretReferences(oldObj.(*hubv1alpha1.AccessControlPolicy))
		newRefs := secretReferences(v)
		w.secretRefCounterMu.Lock()
		for _, ref := range oldRefs {
			if w.secretRefCounter[ref] > 1 {
				w.secretRefCounter[ref]--
			} else {
				delete(w.secretRefCounter, ref)
			}
		}
		for _, ref := range newRefs {
			w.secretRefCounter[ref]++
		}
		w.secretRefCounterMu.Unlock()

	case *corev1.Secret:
		w.secretRefCounterMu.RLock()
		c := w.secretRefCounter[secretKey(v.Name, v.Namespace)]
		w.secretRefCounterMu.RUnlock()
		if c == 0 {
			return
		}

	default:
		log.Error().
			Str("type", fmt.Sprintf("%T", newObj)).
			Msg("Received update event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

// OnDelete implements Kubernetes cache.ResourceEventHandler so it can be used as an informer event handler.
func (w *Watcher) OnDelete(obj interface{}) {
	switch v := obj.(type) {
	case *hubv1alpha1.AccessControlPolicy:
		refs := secretReferences(v)
		w.secretRefCounterMu.Lock()
		for _, ref := range refs {
			if w.secretRefCounter[ref] > 1 {
				w.secretRefCounter[ref]--
			} else {
				delete(w.secretRefCounter, ref)
			}
		}
		w.secretRefCounterMu.Unlock()

	case *corev1.Secret:
		w.secretRefCounterMu.RLock()
		c := w.secretRefCounter[secretKey(v.Name, v.Namespace)]
		w.secretRefCounterMu.RUnlock()
		if c == 0 {
			return
		}

	default:
		log.Error().
			Str("type", fmt.Sprintf("%T", obj)).
			Msg("Received delete event of unknown type")
		return
	}

	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) makeConfigs() (map[string]*acp.Config, error) {
	policies, err := w.acps.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("listing ACPs: %w", err)
	}

	configs := make(map[string]*acp.Config)
	for _, policy := range policies {
		config, err := acp.ConfigFromPolicyWithSecret(policy, w.secrets)
		if err != nil {
			log.Error().
				Err(err).
				Str("acp_name", policy.Name).
				Msg("Could not create ACP configuration")
			continue
		}

		configs[policy.Name] = config
	}

	return configs, nil
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
			logger.Error().Err(err).Msg("Could not Create ACP handler")
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

	case cfg.OAuthIntro != nil:
		return oauthintro.NewHandler(cfg.OAuthIntro, name)

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

	case cfg.OAuthIntro != nil:
		return "OAuth Introspection"

	default:
		return "unknown"
	}
}

func secretKey(name, namespace string) string {
	return name + "@" + namespace
}

func secretReferences(policy *hubv1alpha1.AccessControlPolicy) []string {
	var refs []string

	switch {
	case policy.Spec.OIDC != nil:
		if policy.Spec.OIDC.Secret != nil {
			refs = append(refs, secretKey(policy.Spec.OIDC.Secret.Name, policy.Spec.OIDC.Secret.Namespace))
		}

	case policy.Spec.OIDCGoogle != nil:
		if policy.Spec.OIDCGoogle.Secret != nil {
			refs = append(refs, secretKey(policy.Spec.OIDCGoogle.Secret.Name, policy.Spec.OIDCGoogle.Secret.Namespace))
		}

	case policy.Spec.OAuthIntro != nil:
		refs = append(refs, secretKey(policy.Spec.OAuthIntro.ClientConfig.Auth.Secret.Name, policy.Spec.OAuthIntro.ClientConfig.Auth.Secret.Namespace))
	}

	return refs
}
