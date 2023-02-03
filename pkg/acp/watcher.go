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

package acp

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Client for the ACP service.
type Client interface {
	GetACPs(ctx context.Context) ([]ACP, error)
}

// ACP is the Access Control Policy retrieved from the platform.
type ACP struct {
	Config

	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Watcher watches hub ACPs.
type Watcher struct {
	interval     time.Duration
	client       Client
	hubClientSet hubclientset.Interface
	hubInformer  hubinformer.SharedInformerFactory
}

// NewWatcher returns a new Watcher.
func NewWatcher(interval time.Duration, client Client, hubClientSet hubclientset.Interface, hubInformer hubinformer.SharedInformerFactory) *Watcher {
	return &Watcher{
		interval:     interval,
		client:       client,
		hubClientSet: hubClientSet,
		hubInformer:  hubInformer,
	}
}

// Run runs Watcher.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping ACP watcher")
			return
		case <-t.C:
			ctxFetch, cancel := context.WithTimeout(ctx, 5*time.Second)
			acps, err := w.client.GetACPs(ctxFetch)
			if err != nil {
				log.Error().Err(err).Msg("Fetching ACPs")
				cancel()
				continue
			}
			cancel()

			policies, err := w.hubInformer.Hub().V1alpha1().AccessControlPolicies().Lister().List(labels.Everything())
			if err != nil {
				log.Error().Err(err).Msg("Listing ACPs")
				continue
			}

			policiesByID := map[string]*hubv1alpha1.AccessControlPolicy{}
			for _, p := range policies {
				policiesByID[p.Name] = p
			}

			for _, a := range acps {
				policy, found := policiesByID[a.Name]
				// We delete the policy from the map, since we use this map to delete unused policies.
				delete(policiesByID, a.Name)

				if found && !needUpdate(a, policy) {
					continue
				}

				if !found {
					if err := w.createPolicy(ctx, a); err != nil {
						log.Error().Err(err).Str("name", a.Name).Msg("Creating ACP")
					}
					continue
				}

				if err := w.updatePolicy(ctx, a, policy); err != nil {
					log.Error().Err(err).Str("name", policy.Name).Msg("Upsert ACP")
				}
			}

			w.cleanPolicies(ctx, policiesByID)
		}
	}
}

func (w *Watcher) createPolicy(ctx context.Context, acp ACP) error {
	policy := &hubv1alpha1.AccessControlPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: acp.Name,
		},
		Status: hubv1alpha1.AccessControlPolicyStatus{
			Version: acp.Version,
		},
	}
	policy.Spec = buildAccessControlPolicySpec(acp)

	var err error
	policy.Status.SpecHash, err = policy.Spec.Hash()
	if err != nil {
		return fmt.Errorf("build spec hash: %w ", err)
	}

	ctxCreate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := w.hubClientSet.HubV1alpha1().AccessControlPolicies().Create(ctxCreate, policy, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating ACP: %w", err)
	}
	log.Debug().Str("name", policy.Name).Msg("ACP created")
	return nil
}

func (w *Watcher) updatePolicy(ctx context.Context, acp ACP, policy *hubv1alpha1.AccessControlPolicy) error {
	policy.Spec = buildAccessControlPolicySpec(acp)
	policy.Status.Version = acp.Version

	var err error
	policy.Status.SpecHash, err = policy.Spec.Hash()
	if err != nil {
		return fmt.Errorf("build spec hash: %w", err)
	}

	ctxUpdate, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := w.hubClientSet.HubV1alpha1().AccessControlPolicies().Update(ctxUpdate, policy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating ACP: %w", err)
	}
	log.Debug().Str("name", policy.Name).Msg("ACP updated")

	return nil
}

func (w *Watcher) cleanPolicies(ctx context.Context, policies map[string]*hubv1alpha1.AccessControlPolicy) {
	for _, p := range policies {
		ctxDelete, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := w.hubClientSet.HubV1alpha1().AccessControlPolicies().Delete(ctxDelete, p.Name, metav1.DeleteOptions{})
		if err != nil {
			log.Error().Err(err).Msg("Deleting ACP")
			cancel()
			continue
		}
		log.Debug().Str("name", p.Name).Msg("ACP deleted")
		cancel()
	}
}

func needUpdate(a ACP, policy *hubv1alpha1.AccessControlPolicy) bool {
	return !reflect.DeepEqual(buildAccessControlPolicySpec(a), policy.Spec)
}

func buildAccessControlPolicySpec(a ACP) hubv1alpha1.AccessControlPolicySpec {
	spec := hubv1alpha1.AccessControlPolicySpec{}
	switch {
	case a.JWT != nil:
		spec.JWT = &hubv1alpha1.AccessControlPolicyJWT{
			SigningSecret:              a.JWT.SigningSecret,
			SigningSecretBase64Encoded: a.JWT.SigningSecretBase64Encoded,
			PublicKey:                  a.JWT.PublicKey,
			JWKsFile:                   a.JWT.JWKsFile.String(),
			JWKsURL:                    a.JWT.JWKsURL,
			StripAuthorizationHeader:   a.JWT.StripAuthorizationHeader,
			ForwardHeaders:             a.JWT.ForwardHeaders,
			TokenQueryKey:              a.JWT.TokenQueryKey,
			Claims:                     a.JWT.Claims,
		}

	case a.BasicAuth != nil:
		spec.BasicAuth = &hubv1alpha1.AccessControlPolicyBasicAuth{
			Users:                    a.BasicAuth.Users,
			Realm:                    a.BasicAuth.Realm,
			StripAuthorizationHeader: a.BasicAuth.StripAuthorizationHeader,
			ForwardUsernameHeader:    a.BasicAuth.ForwardUsernameHeader,
		}

	case a.APIKey != nil:
		keys := make([]hubv1alpha1.AccessControlPolicyAPIKeyKey, 0, len(a.APIKey.Keys))
		for _, k := range a.APIKey.Keys {
			keys = append(keys, hubv1alpha1.AccessControlPolicyAPIKeyKey{
				ID:       k.ID,
				Metadata: k.Metadata,
				Value:    k.Value,
			})
		}

		spec.APIKey = &hubv1alpha1.AccessControlPolicyAPIKey{
			Header:         a.APIKey.Header,
			Query:          a.APIKey.Query,
			Cookie:         a.APIKey.Cookie,
			Keys:           keys,
			ForwardHeaders: a.APIKey.ForwardHeaders,
		}

	case a.OIDC != nil:
		spec.OIDC = &hubv1alpha1.AccessControlPolicyOIDC{
			Issuer:         a.OIDC.Issuer,
			ClientID:       a.OIDC.ClientID,
			RedirectURL:    a.OIDC.RedirectURL,
			LogoutURL:      a.OIDC.LogoutURL,
			AuthParams:     a.OIDC.AuthParams,
			Scopes:         a.OIDC.Scopes,
			ForwardHeaders: a.OIDC.ForwardHeaders,
			Claims:         a.OIDC.Claims,
		}

		if a.OIDC.Secret != nil {
			spec.OIDC.Secret = &corev1.SecretReference{
				Name:      a.OIDC.Secret.Name,
				Namespace: a.OIDC.Secret.Namespace,
			}
		}

		if a.OIDC.StateCookie != nil {
			spec.OIDC.StateCookie = &hubv1alpha1.StateCookie{
				SameSite: a.OIDC.StateCookie.SameSite,
				Secure:   a.OIDC.StateCookie.Secure,
				Domain:   a.OIDC.StateCookie.Domain,
				Path:     a.OIDC.StateCookie.Path,
			}
		}

		if a.OIDC.Session != nil {
			spec.OIDC.Session = &hubv1alpha1.Session{
				SameSite: a.OIDC.Session.SameSite,
				Secure:   a.OIDC.Session.Secure,
				Domain:   a.OIDC.Session.Domain,
				Path:     a.OIDC.Session.Path,
				Refresh:  a.OIDC.Session.Refresh,
			}
		}

	case a.OIDCGoogle != nil:
		spec.OIDCGoogle = &hubv1alpha1.AccessControlPolicyOIDCGoogle{
			ClientID:       a.OIDCGoogle.ClientID,
			RedirectURL:    a.OIDCGoogle.RedirectURL,
			LogoutURL:      a.OIDCGoogle.LogoutURL,
			AuthParams:     a.OIDCGoogle.AuthParams,
			ForwardHeaders: a.OIDCGoogle.ForwardHeaders,
			Emails:         a.OIDCGoogle.Emails,
		}

		if a.OIDCGoogle.Secret != nil {
			spec.OIDCGoogle.Secret = &corev1.SecretReference{
				Name:      a.OIDCGoogle.Secret.Name,
				Namespace: a.OIDCGoogle.Secret.Namespace,
			}
		}

		if a.OIDCGoogle.StateCookie != nil {
			spec.OIDCGoogle.StateCookie = &hubv1alpha1.StateCookie{
				SameSite: a.OIDCGoogle.StateCookie.SameSite,
				Secure:   a.OIDCGoogle.StateCookie.Secure,
				Domain:   a.OIDCGoogle.StateCookie.Domain,
				Path:     a.OIDCGoogle.StateCookie.Path,
			}
		}

		if a.OIDCGoogle.Session != nil {
			spec.OIDCGoogle.Session = &hubv1alpha1.Session{
				SameSite: a.OIDCGoogle.Session.SameSite,
				Secure:   a.OIDCGoogle.Session.Secure,
				Domain:   a.OIDCGoogle.Session.Domain,
				Path:     a.OIDCGoogle.Session.Path,
				Refresh:  a.OIDCGoogle.Session.Refresh,
			}
		}
	}

	return spec
}
