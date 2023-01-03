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
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// DomainCache caches the verified domains. It polls the domains
// from the platform at a given interval.
type DomainCache struct {
	client *Client
	ttl    time.Duration

	verifiedMu sync.RWMutex
	verified   []string
}

// NewDomainCache creates a new domain cache configured with
// a platform client and a polling interval.
func NewDomainCache(client *Client, ttl time.Duration) *DomainCache {
	return &DomainCache{client: client, ttl: ttl}
}

// WarmUp feeds the cache by calling the platform to get the
// verified domains. It returns an error for any issue with
// to the platform call.
func (d *DomainCache) WarmUp(ctx context.Context) error {
	return d.updateVerifiedDomains(ctx)
}

// Run starts polling the platform to refresh the cache.
// NOTE: The call is synchronous and could be start in a goroutine.
func (d *DomainCache) Run(ctx context.Context) {
	t := time.NewTicker(d.ttl)

	for {
		select {
		case <-t.C:
			timeoutCtx, cancelFunc := context.WithTimeout(ctx, d.ttl)
			if err := d.updateVerifiedDomains(timeoutCtx); err != nil {
				log.Error().Err(err).Msg("unable to list verified domains")
			}

			cancelFunc()
		case <-ctx.Done():
			log.Error().Err(ctx.Err()).Msg("stop listing verified domains")
			return
		}
	}
}

func (d *DomainCache) updateVerifiedDomains(ctx context.Context) error {
	domains, err := d.client.ListVerifiedDomains(ctx)
	if err != nil {
		return err
	}

	d.verifiedMu.Lock()
	defer d.verifiedMu.Unlock()

	d.verified = domains
	return nil
}

// ListVerifiedDomains implements the validationwebhook.DomainLister interface.
func (d *DomainCache) ListVerifiedDomains(_ context.Context) []string {
	d.verifiedMu.RLock()
	defer d.verifiedMu.RUnlock()

	return d.verified
}
