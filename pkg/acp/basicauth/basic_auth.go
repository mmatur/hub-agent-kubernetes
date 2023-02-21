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

package basicauth

import (
	"fmt"
	"net/http"
	"strings"

	goauth "github.com/abbot/go-http-auth"
	"github.com/rs/zerolog/log"
)

const defaultRealm = "hub"

// Users holds a list of users.
type Users []string

// Config configures a basic auth ACP handler.
type Config struct {
	Users                    Users  `json:"users,omitempty"`
	Realm                    string `json:"realm,omitempty"`
	StripAuthorizationHeader bool   `json:"stripAuthorizationHeader,omitempty"`
	ForwardUsernameHeader    string `json:"forwardUsernameHeader,omitempty"`
}

// Handler is a basic auth ACP Handler.
type Handler struct {
	auth               *goauth.BasicAuth
	users              map[string]string
	forwardUsername    string
	stripAuthorization bool
	name               string
}

// NewHandler creates a new basic auth ACP Handler.
func NewHandler(cfg *Config, name string) (*Handler, error) {
	users, err := getUsers(cfg.Users, basicUserParser)
	if err != nil {
		return nil, err
	}

	h := &Handler{
		users:              users,
		forwardUsername:    cfg.ForwardUsernameHeader,
		stripAuthorization: cfg.StripAuthorizationHeader,
		name:               name,
	}

	realm := defaultRealm
	if len(cfg.Realm) > 0 {
		realm = cfg.Realm
	}

	h.auth = &goauth.BasicAuth{Realm: realm, Secrets: h.secretBasic}

	return h, nil
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	l := log.With().Str("handler_type", "BasicAuth").Str("handler_name", h.name).Logger()

	username, password, ok := req.BasicAuth()
	if ok {
		secret := h.auth.Secrets(username, h.auth.Realm)
		if secret == "" || !goauth.CheckSecret(password, secret) {
			ok = false
		}
	}

	if !ok {
		l.Debug().Msg("Authentication failed")

		h.auth.RequireAuth(rw, req)
		return
	}

	if h.forwardUsername != "" {
		rw.Header().Set(h.forwardUsername, username)
	}

	if h.stripAuthorization {
		rw.Header().Add("Authorization", "")
	}

	rw.WriteHeader(http.StatusOK)
}

func (h *Handler) secretBasic(user, _ string) string {
	if secret, ok := h.users[user]; ok {
		return secret
	}

	return ""
}

func basicUserParser(user string) (username, password string, err error) {
	split := strings.Split(user, ":")
	if len(split) != 2 {
		return "", "", fmt.Errorf("parse BasicUser: %v", user)
	}
	return split[0], split[1], nil
}

// userParser Parses a string and return a userName/userHash. An error if the format of the string is incorrect.
type userParser func(user string) (username, password string, err error)

func getUsers(users []string, parser userParser) (map[string]string, error) {
	userMap := make(map[string]string)
	for _, user := range users {
		userName, userHash, err := parser(user)
		if err != nil {
			return nil, err
		}
		userMap[userName] = userHash
	}

	return userMap, nil
}
