package tunnel

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_updateTunnels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wait := make(chan struct{})
	ingCtrlServiceURL := createIngCtrlService(t, wait, "cTunnel", "nTunnel", "sTunnel")

	currentBroker := buildBroker(t, []byte("cTunnel"), "current-tunnel")
	currentBrokerURL, err := url.Parse(currentBroker.URL)
	require.NoError(t, err)

	newBroker := buildBroker(t, []byte("nTunnel"), "new-tunnel")
	newBrokerURL, err := url.Parse(newBroker.URL)
	require.NoError(t, err)

	stableBroker := buildBroker(t, []byte("sTunnel"), "stable-tunnel")
	stableBrokerURL, err := url.Parse(stableBroker.URL)
	require.NoError(t, err)

	client := &clientMock{
		listClusterTunnelEndpoints: func() ([]Endpoint, error) {
			return []Endpoint{
				{
					TunnelID:        "current-tunnel",
					BrokerEndpoint:  "ws://" + currentBrokerURL.Host,
					ClusterEndpoint: ingCtrlServiceURL,
				},
				{
					TunnelID:        "new-tunnel",
					BrokerEndpoint:  "ws://" + newBrokerURL.Host,
					ClusterEndpoint: ingCtrlServiceURL,
				},
				{
					TunnelID:        "stable-tunnel",
					BrokerEndpoint:  "ws://" + stableBrokerURL.Host,
					ClusterEndpoint: ingCtrlServiceURL,
				},
			}, nil
		},
	}

	c := fakeClient(t)
	manager := NewManager(client, "token")
	manager.tunnels["current-tunnel-new-broker"] = &tunnel{
		BrokerEndpoint:  "old-endpoint",
		ClusterEndpoint: "old-endpoint",
		Client:          &closeAwareListener{Listener: c},
	}
	manager.tunnels["unused-tunnel"] = &tunnel{
		BrokerEndpoint:  "old-endpoint",
		ClusterEndpoint: "old-endpoint",
		Client:          &closeAwareListener{Listener: c},
	}

	stopped := make(chan struct{})
	go func() {
		manager.Run(ctx)
		close(stopped)
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	case <-wait:
	}

	manager.tunnelsMu.Lock()
	assert.Len(t, manager.tunnels, 3)
	assert.Equal(t, "ws://"+currentBrokerURL.Host, manager.tunnels["current-tunnel"].BrokerEndpoint)
	assert.Equal(t, ingCtrlServiceURL, manager.tunnels["current-tunnel"].ClusterEndpoint)
	assert.Equal(t, "ws://"+newBrokerURL.Host, manager.tunnels["new-tunnel"].BrokerEndpoint)
	assert.Equal(t, ingCtrlServiceURL, manager.tunnels["new-tunnel"].ClusterEndpoint)
	assert.Equal(t, "ws://"+stableBrokerURL.Host, manager.tunnels["stable-tunnel"].BrokerEndpoint)
	assert.Equal(t, ingCtrlServiceURL, manager.tunnels["stable-tunnel"].ClusterEndpoint)

	manager.tunnelsMu.Unlock()

	// stop the manager.
	cancel()

	// wait for the manager to stop.
	<-stopped

	manager.tunnelsMu.Lock()
	assert.Len(t, manager.tunnels, 0)
	manager.tunnelsMu.Unlock()
}

func createIngCtrlService(t *testing.T, wait chan struct{}, messages ...string) string {
	t.Helper()

	ingCtrlService, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "0"))
	require.NoError(t, err)

	received := map[string]struct{}{}
	go func() {
		for {
			inboundConn, aErr := ingCtrlService.Accept()
			require.NoError(t, aErr)

			b := make([]byte, 7)
			_, err := inboundConn.Read(b)
			require.NoError(t, err)

			if contains(messages, string(b)) {
				received[string(b)] = struct{}{}
			}

			if len(received) == len(messages) {
				close(wait)
			}
		}
	}()

	return ingCtrlService.Addr().String()
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}

func fakeClient(t *testing.T) *yamux.Session {
	t.Helper()

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	c, err := yamux.Client(&readWriteCloseMock{}, cfg)
	require.NoError(t, err)

	return c
}

func buildBroker(t *testing.T, message []byte, tunnelID string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		hdr := req.Header.Get("Authorization")
		assert.Equal(t, "Bearer token", hdr)
		assert.Equal(t, "/"+tunnelID, req.URL.Path)

		upgrader := &websocket.Upgrader{}
		websocketConn, err := upgrader.Upgrade(rw, req, nil)
		assert.NoError(t, err)

		w := &websocketNetConn{
			Conn: websocketConn,
		}

		cfg := yamux.DefaultConfig()
		cfg.LogOutput = io.Discard
		server, err := yamux.Server(w, cfg)
		assert.NoError(t, err)

		con, err := server.Open()
		assert.NoError(t, err)

		for {
			select {
			case <-req.Context().Done():
				return
			default:
				time.Sleep(time.Millisecond)
				_, _ = con.Write(message)
			}
		}
	}))
}

type clientMock struct {
	listClusterTunnelEndpoints func() ([]Endpoint, error)
}

func (c *clientMock) ListClusterTunnelEndpoints(_ context.Context) ([]Endpoint, error) {
	return c.listClusterTunnelEndpoints()
}

type readWriteCloseMock struct {
	closedMu sync.Mutex
	closed   bool
}

func (r *readWriteCloseMock) Read(_ []byte) (n int, err error) {
	r.closedMu.Lock()
	defer r.closedMu.Unlock()

	if r.closed {
		return 0, io.EOF
	}

	return 0, nil
}

func (r *readWriteCloseMock) Write(_ []byte) (n int, err error) {
	return
}

func (r *readWriteCloseMock) Close() error {
	r.closedMu.Lock()
	r.closed = true
	r.closedMu.Unlock()

	return nil
}
