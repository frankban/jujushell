// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wsproxy

import "io"

// Copy copies messages back and forth between the provided WebSocket
// connections.
func Copy(conn1, conn2 Conn) error {
	// Start copying WebSocket messages back and forth.
	errCh := make(chan error, 2)
	go cp(conn1, conn2, errCh)
	go cp(conn2, conn1, errCh)
	return <-errCh
}

// cp copies all frames sent from the src WebSocket connection to the dst one,
// and sends errors to the given error channel.
func cp(dst, src Conn, errCh chan error) {
	var err error
	for {
		if err = copyMessage(dst, src); err != nil {
			errCh <- err
			return
		}
	}
}

// copyMessage copies a single message frame sent by src to dst.
func copyMessage(dst, src Conn) error {
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
	return w.Close()
}

// Conn is a WebSocket connection that can be managed through data message
// readers and writers.
type Conn interface {
	// NextReader returns the next data message received from the peer. The
	// returned messageType is either TextMessage or BinaryMessage.
	NextReader() (messageType int, r io.Reader, err error)
	// NextWriter returns a writer for the next message to send. The writer's
	// Close method flushes the complete message to the network.
	NextWriter(messageType int) (io.WriteCloser, error)
}
