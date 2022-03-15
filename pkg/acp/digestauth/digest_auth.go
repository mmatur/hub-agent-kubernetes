package digestauth

import (
	"fmt"
	"net/http"
	"strings"

	goauth "github.com/abbot/go-http-auth"
	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/basicauth"
)

const defaultRealm = "hub"

// Config is the configuration of a digest auth ACP handler.
type Config struct {
	Users                    basicauth.Users
	Realm                    string
	StripAuthorizationHeader bool
	ForwardUsernameHeader    string
}

// Handler is a digest auth ACP Handler.
type Handler struct {
	auth               *goauth.DigestAuth
	users              map[string]string
	forwardUsername    string
	stripAuthorization bool
	name               string
}

// NewHandler creates a new digest auth ACP Handler.
func NewHandler(cfg *Config, name string) (*Handler, error) {
	users, err := getUsers(cfg.Users, digestUserParser)
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
	h.auth = goauth.NewDigestAuthenticator(realm, h.secretDigest)

	return h, nil
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	l := log.With().Str("handler_type", "DigestAuth").Str("handler_name", h.name).Logger()

	username, info := h.auth.CheckAuth(req)
	if username == "" {
		if info != nil && *info == "stale" {
			l.Debug().Msg("Digest authentication failed, possibly because out of order requests")
			h.auth.RequireAuthStale(rw, req)
			return
		}

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

func (h *Handler) secretDigest(user, realm string) string {
	if secret, ok := h.users[user+":"+realm]; ok {
		return secret
	}

	return ""
}

func digestUserParser(user string) (username, password string, err error) {
	split := strings.Split(user, ":")
	if len(split) != 3 {
		return "", "", fmt.Errorf("error parsing DigestUser: %v", user)
	}
	return split[0] + ":" + split[1], split[2], nil
}

// userParser Parses a string and return a userName/userHash. An error if the format of the string is incorrect.
type userParser func(user string) (string, string, error)

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
