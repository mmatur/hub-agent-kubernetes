package tunnel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebsocketNetConn_Read(t *testing.T) {
	timeout := 10 * time.Millisecond

	serverConnCh := make(chan *websocket.Conn)
	var upgrader websocket.Upgrader

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		serverConn, _ := upgrader.Upgrade(rw, req, nil)
		serverConnCh <- serverConn
	}))
	defer t.Cleanup(server.Close)

	url := "ws://" + strings.TrimPrefix(server.URL, "http://")
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)

	var serverConn *websocket.Conn
	select {
	case <-time.After(timeout):
		require.Fail(t, "timed out while waiting for a server connection")
	case serverConn = <-serverConnCh:
	}

	finishedReadingCh := make(chan struct{})
	go func() {
		if serverConn == nil {
			require.Fail(t, "server connection is nil")
			return
		}
		sErr := serverConn.WriteMessage(websocket.BinaryMessage, []byte("hello!"))
		require.NoError(t, sErr)

		select {
		case <-finishedReadingCh:
		case <-time.After(timeout):
			require.Fail(t, "timed out waiting for read to finish")
		}

		require.NoError(t, serverConn.Close())
	}()

	conn := websocketNetConn{Conn: wsConn}
	require.NoError(t, conn.SetDeadline(time.Now().Add(timeout)))

	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	require.NoError(t, err)

	assert.Equal(t, 4, n)
	assert.Equal(t, []byte("hell"), buf)
	n, err = conn.Read(buf)
	require.NoError(t, err)

	assert.Equal(t, 2, n)
	assert.Equal(t, []byte("o!"), buf[:n])

	close(finishedReadingCh)

	_, err = conn.Read(buf)
	require.Error(t, err)
	assert.True(t, websocket.IsUnexpectedCloseError(err))
}

func TestWebsocketNetConn_Write(t *testing.T) {
	timeout := 10 * time.Millisecond

	serverConnCh := make(chan *websocket.Conn)
	var upgrader websocket.Upgrader

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		serverConn, _ := upgrader.Upgrade(rw, req, nil)
		serverConnCh <- serverConn
	}))
	defer t.Cleanup(server.Close)

	url := "ws://" + strings.TrimPrefix(server.URL, "http://")
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)

	var serverConn *websocket.Conn
	select {
	case <-time.After(timeout):
		require.Fail(t, "timed out while waiting for a server connection")
	case serverConn = <-serverConnCh:
	}

	conn := websocketNetConn{Conn: wsConn}
	require.NoError(t, conn.SetDeadline(time.Now().Add(timeout)))

	_, err = conn.Write([]byte("hello!"))
	require.NoError(t, err)

	if serverConn == nil {
		require.Fail(t, "server connection is nil")
		return
	}
	typ, buf, err := serverConn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, []byte("hello!"), buf)
	assert.Equal(t, websocket.BinaryMessage, typ)
}
