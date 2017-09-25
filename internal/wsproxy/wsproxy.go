package wsproxy

import (
	"io"

	"github.com/gorilla/websocket"
)

// Copy copies messages back and forth between the provided WebSocket
// connections.
func Copy(conn1, conn2 *websocket.Conn) error {
	// Start copying WebSocket messages back and forth.
	errCh := make(chan error, 2)
	go cp(conn1, conn2, errCh)
	go cp(conn2, conn1, errCh)
	return <-errCh
}

// cp copies all frames sent from the src WebSocket connection to the dst one,
// and sends errors to the given error channel.
func cp(dst, src *websocket.Conn, errCh chan error) {
	var err error
	for {
		if err = copyMessage(dst, src); err != nil {
			errCh <- err
			return
		}
	}
}

// copyMessage copies a single message frame sent by src to dst.
func copyMessage(dst, src *websocket.Conn) error {
	messageType, r, err := src.NextReader()
	if err != nil {
		return err
	}
	w, err := dst.NextWriter(messageType)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}
