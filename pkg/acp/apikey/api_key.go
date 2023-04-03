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

package apikey

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/token"
	"golang.org/x/crypto/sha3"
)

// Config configures an API key ACP handler.
type Config struct {
	KeySource      token.Source      `json:"keySource"`
	Keys           []Key             `json:"keys"`
	ForwardHeaders map[string]string `json:"forwardHeaders"`
}

// Key defines an API key.
type Key struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata"`
	Value    string            `json:"value"`
}

// Handler is an API Key ACP Handler.
type Handler struct {
	name       string
	keySrc     token.Source
	keys       map[string]Key
	fwdHeaders map[string]string
}

// NewHandler creates a new API key ACP Handler.
func NewHandler(cfg *Config, name string) (*Handler, error) {
	if cfg.KeySource.Header == "" && cfg.KeySource.Query == "" && cfg.KeySource.Cookie == "" {
		return nil, errors.New(`at least one of "header", "query" or "cookie" must be set`)
	}

	if len(cfg.Keys) == 0 {
		return nil, errors.New("at least one key must be defined")
	}

	keys := make(map[string]Key, len(cfg.Keys))
	uniqIDs := make(map[string]struct{}, len(cfg.Keys))
	uniqValues := make(map[string]struct{}, len(cfg.Keys))
	for _, k := range cfg.Keys {
		if k.ID == "" || k.Value == "" {
			return nil, errors.New("empty ID or value")
		}

		if _, ok := uniqIDs[k.ID]; ok {
			return nil, fmt.Errorf("duplicated key ID %q", k.ID)
		}
		uniqIDs[k.ID] = struct{}{}

		if _, ok := uniqValues[k.Value]; ok {
			return nil, fmt.Errorf("duplicated key value %q", k.Value)
		}
		uniqValues[k.Value] = struct{}{}

		md := make(map[string]string, len(k.Metadata)+1)
		for mk, mv := range k.Metadata {
			md[mk] = mv
		}
		// Key ID is not part of metadata, add is under the "_id" key.
		md["_id"] = k.ID

		keys[k.Value] = Key{
			ID:       k.ID,
			Metadata: md,
			Value:    k.Value,
		}
	}

	return &Handler{
		name:       name,
		keySrc:     cfg.KeySource,
		keys:       keys,
		fwdHeaders: cfg.ForwardHeaders,
	}, nil
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	l := log.With().Str("handler_type", "APIKey").Str("handler_name", h.name).Logger()

	apiKey, err := token.Extract(req, h.keySrc)
	if err != nil {
		l.Debug().Err(err).Msg("Getting API key")
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	hash := make([]byte, 64)
	sha3.ShakeSum256(hash, []byte(apiKey))
	k, ok := h.keys[fmt.Sprintf("%x", hash)]
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	queryParam := req.URL.Query()
	if queryParam.Get("groups") != "" {
		groups, err := url.QueryUnescape(queryParam.Get("groups"))
		if err != nil {
			log.Error().Err(err).Msg("Error while unescaping groups")

			rw.WriteHeader(http.StatusInternalServerError)
			return
		}

		userGroupsRaw, ok := k.Metadata["groups"]
		if !ok {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}

		userGroups := strings.Split(userGroupsRaw, ",")

		var found bool
		for _, group := range strings.Split(groups, ",") {
			if search(group, userGroups) {
				found = true
				break
			}
		}

		if !found {
			log.Debug().
				Str("groups", groups).
				Strs("user_groups", userGroups).
				Msg("User is not in the required groups")

			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	for name, meta := range h.fwdHeaders {
		if v, exists := k.Metadata[meta]; exists {
			rw.Header().Add(name, v)
		}
	}

	rw.WriteHeader(http.StatusOK)
}

func search(needle string, stack []string) bool {
	for _, s := range stack {
		if s == needle {
			return true
		}
	}

	return false
}
