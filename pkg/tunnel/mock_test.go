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
	"io"
	"sync"
)

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
