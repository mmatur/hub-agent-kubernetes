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

package jwt

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pquerna/cachecontrol"
	"gopkg.in/square/go-jose.v2"
)

// KeySet allows to get a signing key from a JWK set.
type KeySet interface {
	Key(ctx context.Context, keyID string) (*jose.JSONWebKey, error)
}

// ContentKeySet gets signing keys from a JWK set given as raw content.
type ContentKeySet struct {
	keySet *jose.JSONWebKeySet
}

// NewContentKeySet returns a ContentKeySet.
func NewContentKeySet(content []byte) (*ContentKeySet, error) {
	var keySet jose.JSONWebKeySet
	if err := json.Unmarshal(content, &keySet); err != nil {
		return nil, fmt.Errorf("unable to decode JWK set from content: %w", err)
	}

	return &ContentKeySet{
		keySet: &keySet,
	}, nil
}

// Key returns a key for a given key ID.
func (k *ContentKeySet) Key(_ context.Context, keyID string) (*jose.JSONWebKey, error) {
	keys := k.keySet.Key(keyID)
	if len(keys) == 0 {
		return nil, nil
	}
	return &keys[0], nil
}

// FileKeySet gets signing keys from a JWK set stored in a file.
type FileKeySet struct {
	mu sync.RWMutex

	path string
	// Actual mod time of the path.
	lastModTime time.Time
	// Time at which we last checked the mod time of the path.
	// Used to avoid having to stat the path too often.
	lastCheck time.Time
	// Interval at which we should check the modTime of the file.
	checkInterval time.Duration

	keySet *jose.JSONWebKeySet
}

// NewFileKeySet returns a FileKeySet.
func NewFileKeySet(path string) *FileKeySet {
	return &FileKeySet{
		path:          path,
		checkInterval: 5 * time.Second,
	}
}

// Key returns a key for a given key ID.
func (k *FileKeySet) Key(_ context.Context, keyID string) (*jose.JSONWebKey, error) {
	if err := k.updateKeySet(); err != nil {
		return nil, err
	}

	k.mu.RLock()
	defer k.mu.RUnlock()

	keys := k.keySet.Key(keyID)
	if len(keys) == 0 {
		return nil, nil
	}
	return &keys[0], nil
}

func (k *FileKeySet) readKeySet() error {
	f, err := os.Open(k.path)
	if err != nil {
		return fmt.Errorf("unable to open key set file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var keySet jose.JSONWebKeySet
	if err = json.NewDecoder(f).Decode(&keySet); err != nil {
		return fmt.Errorf("unable to decode key set file: %w", err)
	}

	k.keySet = &keySet

	return nil
}

func (k *FileKeySet) isExpired() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()

	return k.lastCheck.Add(k.checkInterval).Before(time.Now())
}

func (k *FileKeySet) updateKeySet() error {
	if !k.isExpired() {
		return nil
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if k.lastCheck.Add(k.checkInterval).After(time.Now()) {
		return nil
	}

	info, err := os.Stat(k.path)
	if err != nil {
		return fmt.Errorf("unable to stat key set file: %w", err)
	}

	if !k.lastModTime.Equal(info.ModTime()) {
		if err = k.readKeySet(); err != nil {
			return fmt.Errorf("unable to read path key set: %w", err)
		}

		k.lastModTime = info.ModTime()
	}

	k.lastCheck = time.Now()

	return nil
}

// RemoteKeySet resolves a key set based on a key set URL, and keeps it up to date.
type RemoteKeySet struct {
	url string

	mu       sync.RWMutex
	keys     jose.JSONWebKeySet
	expiry   time.Time
	updating *inflight
	client   *http.Client
}

// NewRemoteKeySet returns a RemoteKeySet.
func NewRemoteKeySet(url string) *RemoteKeySet {
	return &RemoteKeySet{
		url: url,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
			},
			Timeout: 5 * time.Second,
		},
	}
}

// Key returns a key for a given key ID.
func (s *RemoteKeySet) Key(ctx context.Context, keyID string) (*jose.JSONWebKey, error) {
	if err := s.updateKeySet(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := s.keys.Key(keyID)
	if len(keys) == 0 {
		return nil, nil
	}
	return &keys[0], nil
}

func (s *RemoteKeySet) updateKeySet(ctx context.Context) error {
	if !s.isExpired() {
		return nil
	}

	s.mu.Lock()
	if s.updating == nil {
		s.updating = newInflight()

		go func() {
			keySet, expiry, err := fetchKeys(ctx, s.client, s.url)

			s.mu.Lock()
			defer s.mu.Unlock()

			if err == nil {
				s.keys = *keySet
				s.expiry = expiry
			}

			s.updating.Done(err)
			s.updating = nil
		}()
	}

	updating := s.updating
	s.mu.Unlock()

	return updating.Wait(ctx)
}

func (s *RemoteKeySet) isExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return time.Now().After(s.expiry)
}

func fetchKeys(ctx context.Context, client *http.Client, url string) (*jose.JSONWebKeySet, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("unable to build fetch keys request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("unable to fetch keys: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("unexpected status code %q", resp.Status)
	}

	var keySet jose.JSONWebKeySet
	if err = json.NewDecoder(resp.Body).Decode(&keySet); err != nil {
		return nil, time.Time{}, fmt.Errorf("unable to decode body: %w", err)
	}

	// If the server doesn't provide cache control headers, assume the
	// keys expire immediately.
	expiry := time.Now()
	_, e, err := cachecontrol.CachableResponse(req, resp, cachecontrol.Options{})
	if err == nil && e.After(expiry) {
		expiry = e
	}

	return &keySet, expiry, nil
}

type inflight struct {
	ch  chan struct{}
	err error
}

func newInflight() *inflight {
	return &inflight{
		ch: make(chan struct{}),
	}
}

func (i *inflight) Wait(ctx context.Context) error {
	select {
	case <-i.ch:
		return i.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *inflight) Done(err error) {
	i.err = err
	close(i.ch)
}
