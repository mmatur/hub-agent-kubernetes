package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	log.Logger = zerolog.New(io.Discard).With().Caller().Logger()
	code := m.Run()
	os.Exit(code)
}

func TestConfigWatcher_Run(t *testing.T) {
	cfg := Config{
		AccessControl: AccessControlConfig{MaxSecuredRoutes: 1},
	}

	client := setupClient(t, cfg)
	configWatcher := NewConfigWatcher(time.Millisecond, client)

	wait := make(chan struct{})
	var gotCfg Config
	listener := func(cfg Config) {
		gotCfg = cfg
		close(wait)
	}
	configWatcher.AddListener(listener)

	ctx := context.Background()
	go configWatcher.Run(ctx)

	<-wait

	assert.Equal(t, cfg, gotCfg)
}

func TestConfigWatcher_RunHandlesSIGHUP(t *testing.T) {
	cfg := Config{
		AccessControl: AccessControlConfig{MaxSecuredRoutes: 1},
	}
	client := setupClient(t, cfg)
	configWatcher := NewConfigWatcher(time.Hour, client)

	wait := make(chan struct{})
	var gotCfg Config
	var l sync.RWMutex
	var closed bool
	listener := func(cfg Config) {
		gotCfg = cfg

		l.Lock()
		if !closed {
			close(wait)
			closed = true
		}
		l.Unlock()
	}
	configWatcher.AddListener(listener)

	ctx := context.Background()
	go configWatcher.Run(ctx)

	go func() {
		for {
			l.RLock()
			if closed {
				l.RUnlock()
				return
			}
			l.RUnlock()

			pid := os.Getpid()
			err := syscall.Kill(pid, syscall.SIGHUP)
			require.NoError(t, err)
		}
	}()

	<-wait

	assert.Equal(t, cfg, gotCfg)
}

func setupClient(t *testing.T, cfg Config) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		err := json.NewEncoder(rw).Encode(cfg)
		require.NoError(t, err)
	})

	srv := httptest.NewServer(mux)

	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "123")
	client.httpClient = srv.Client()

	return client
}
