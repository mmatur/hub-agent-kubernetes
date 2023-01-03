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

package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"github.com/rs/zerolog/log"
)

// Backend is able to call hub-tunnel API.
type Backend interface {
	ListClusterTunnelEndpoints(ctx context.Context) ([]Endpoint, error)
}

// Manager manages tunnels.
type Manager struct {
	client            Backend
	token             string
	traefikTunnelAddr string

	tunnelsMu sync.Mutex
	tunnels   map[string]*tunnel
}

type tunnel struct {
	BrokerEndpoint  string
	ClusterEndpoint string
	Client          *closeAwareListener
}

func (t *tunnel) Close() error {
	if t.Client != nil {
		return t.Client.Close()
	}

	return nil
}

// NewManager returns a new manager instance.
func NewManager(tunnels Backend, traefikTunnelAddr, token string) Manager {
	return Manager{
		client:            tunnels,
		traefikTunnelAddr: traefikTunnelAddr,
		token:             token,
		tunnels:           make(map[string]*tunnel),
	}
}

// Run runs the manager.
// While running, the manager fetches every minute the tunnels available for
// this cluster and create/delete tunnels accordingly.
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	if err := m.updateTunnels(ctx); err != nil {
		log.Error().Err(err).Msg("Unable to update tunnels")
	}

	for {
		select {
		case <-ticker.C:
			if err := m.updateTunnels(ctx); err != nil {
				log.Error().Err(err).Msg("Unable to update tunnels")
				continue
			}

		case <-ctx.Done():
			m.stop()
			return
		}
	}
}

func (m *Manager) stop() {
	m.tunnelsMu.Lock()
	defer m.tunnelsMu.Unlock()

	for id, tun := range m.tunnels {
		if err := tun.Close(); err != nil {
			log.Error().Err(err).
				Str("tunnel_id", id).
				Msg("Unable to close tunnel")
		}
		delete(m.tunnels, id)
	}
}

func (m *Manager) updateTunnels(ctx context.Context) error {
	m.tunnelsMu.Lock()
	defer m.tunnelsMu.Unlock()

	endpoints, err := m.client.ListClusterTunnelEndpoints(ctx)
	if err != nil {
		return fmt.Errorf("unable to list tunnels: %w", err)
	}

	currentTunnels := make(map[string]struct{})
	for _, endpoint := range endpoints {
		logger := log.With().
			Str("broker_endpoint", endpoint.BrokerEndpoint).
			Str("tunnel_id", endpoint.TunnelID).
			Logger()
		currentTunnels[endpoint.TunnelID] = struct{}{}

		tun, found := m.tunnels[endpoint.TunnelID]
		if !found {
			m.launchTunnel(endpoint)

			continue
		}

		if tun.BrokerEndpoint != endpoint.BrokerEndpoint {
			if err = tun.Close(); err != nil {
				logger.Error().Err(err).Msg("Unable to close tunnel")
			}
			delete(m.tunnels, endpoint.TunnelID)

			m.launchTunnel(endpoint)
		}
	}

	for id := range m.tunnels {
		if _, found := currentTunnels[id]; !found {
			if err = m.tunnels[id].Close(); err != nil {
				log.Error().Err(err).Msg("Unable to close tunnel")
			}
			delete(m.tunnels, id)
		}
	}

	return nil
}

func (m *Manager) launchTunnel(endpoint Endpoint) {
	t := &tunnel{BrokerEndpoint: endpoint.BrokerEndpoint, ClusterEndpoint: m.traefikTunnelAddr}
	m.tunnels[endpoint.TunnelID] = t

	go func(t *tunnel, tunnelID string) {
		err := t.launch(tunnelID, m.token)
		if err != nil {
			log.Error().Err(err).Msg("Launch tunnel")
		}

		m.tunnelsMu.Lock()
		delete(m.tunnels, tunnelID)
		m.tunnelsMu.Unlock()
	}(t, endpoint.TunnelID)
}

func (t *tunnel) launch(tunnelID, token string) error {
	u, err := url.Parse(t.BrokerEndpoint)
	if err != nil {
		return fmt.Errorf("parse broker endpoint: %w", err)
	}
	u.Path = path.Join(u.Path, tunnelID)

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 30 * time.Second,
	}
	connSocket, resp, err := dialer.Dial(u.String(), http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("expected protocol switching, got: %d", resp.StatusCode)
	}

	conn := &websocketNetConn{
		Conn: connSocket,
	}

	cfg := &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		StreamOpenTimeout:      75 * time.Second,
		StreamCloseTimeout:     5 * time.Minute,
		LogOutput:              io.Discard,
	}
	client, err := yamux.Client(conn, cfg)
	if err != nil {
		return fmt.Errorf("new yamux client: %w", err)
	}

	t.Client = &closeAwareListener{Listener: client}

	for {
		brokerConn, acceptErr := t.Client.Accept()
		if acceptErr != nil {
			if errors.Is(acceptErr, errListenerClosed) {
				return nil
			}

			return fmt.Errorf("accept: %w", acceptErr)
		}

		go func(brokerConn net.Conn) {
			if err = proxy(brokerConn, t.ClusterEndpoint); err != nil {
				log.Error().Err(err).Msg("Unable to proxy the tunnel traffic to the cluster endpoint")
			}
		}(brokerConn)
	}
}

func proxy(sourceConn net.Conn, addr string) error {
	targetConn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	errCh := make(chan error)

	go connCopy(errCh, targetConn, sourceConn)
	go connCopy(errCh, sourceConn, targetConn)

	err = <-errCh
	<-errCh

	if err != nil {
		return fmt.Errorf("copy conn: %w", err)
	}

	return nil
}

func connCopy(errCh chan<- error, dst io.WriteCloser, src io.Reader) {
	_, err := io.Copy(dst, src)
	errCh <- err

	if err = dst.Close(); err != nil {
		log.Error().Err(err).Msg("Unable to close destination connection")
	}
}

// closeAwareListener provides a listener that triggers an error when the connection is closed. net.Listener use
// to return a "use of closed network connection" error when the connection is closed. As suggested in
// https://github.com/golang/go/issues/4373, the wrapper captures the close order and serves a sentinel that
// can be safely caught.
type closeAwareListener struct {
	net.Listener

	closed   bool
	closedMu sync.RWMutex
}

var errListenerClosed = errors.New("listener closed")

func (l *closeAwareListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		l.closedMu.RLock()
		defer l.closedMu.RUnlock()

		if l.closed || errors.Is(err, io.EOF) {
			return nil, errListenerClosed
		}

		return nil, err
	}

	return conn, nil
}

func (l *closeAwareListener) Close() error {
	l.closedMu.Lock()
	l.closed = true
	l.closedMu.Unlock()

	return l.Listener.Close()
}
