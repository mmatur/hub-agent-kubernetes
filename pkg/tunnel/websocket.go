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
