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

package tunnel

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// websocketNetConn wraps a websocket.Conn and exposes it as a net.Conn.
type websocketNetConn struct {
	*websocket.Conn

	buff []byte
}

// Read reads data from the connection. This method is not thread-safe, multiple read shouldn't
// be attempted simultaneously.
func (c *websocketNetConn) Read(dst []byte) (int, error) {
	// Read on the connection if there's nothing left in the buffer.
	if len(c.buff) == 0 {
		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			return 0, err
		}

		c.buff = msg
	}

	// Copy as much as possible from the buffer to dst and keep the rest in the buffer.
	n := copy(dst, c.buff)
	c.buff = c.buff[n:]

	return n, nil
}

// Write writes data to the connection.
func (c *websocketNetConn) Write(b []byte) (int, error) {
	if err := c.Conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
		return 0, err
	}

	return len(b), nil
}

// SetDeadline sets the read and write deadlines.
func (c *websocketNetConn) SetDeadline(t time.Time) error {
	if err := c.Conn.SetReadDeadline(t); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	if err := c.Conn.SetWriteDeadline(t); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}

	return nil
}
