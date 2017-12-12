// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wstransport

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	errgo "gopkg.in/errgo.v1"

	"github.com/juju/jujushell/apiparams"
	"github.com/juju/jujushell/internal/logging"
)

var log = logging.Log()

// Conn describes a WebSocket connection.
type Conn interface {
	// ReadJSON reads the next JSON-encoded message from the connection and
	// stores it in the value pointed to by v.
	ReadJSON(v interface{}) error
	// WriteJSON writes the JSON encoding of v as a message.
	WriteJSON(v interface{}) error
	// NextReader returns the next data message received from the peer. The
	// returned messageType is either TextMessage or BinaryMessage.
	NextReader() (messageType int, r io.Reader, err error)
	// NextWriter returns a writer for the next message to send. The writer's
	// Close method flushes the complete message to the network.
	NextWriter(messageType int) (io.WriteCloser, error)
	// Error writes an error response including the given error as a message.
	// The error is also returned.
	Error(err error) error
	// OK writes a success response with the given formatted text as a message.
	OK(format string, a ...interface{}) error
	// Close closes the WebSocket connection.
	Close() error
}

// Upgrade upgrades the HTTP server connection to the WebSocket protocol.
// If the upgrade fails, then Upgrade replies to the client with an HTTP error.
func Upgrade(w http.ResponseWriter, r *http.Request) (Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &connection{
		Conn: conn,
	}, nil
}

// connection implements Conn.
type connection struct {
	*websocket.Conn
}

// Error implements conn.Error by sending a JSON message with the given error.
func (conn *connection) Error(err error) error {
	if werr := writeResponse(conn, apiparams.Error, err.Error()); werr != nil {
		return errgo.Notef(werr, "original error: %v", err)
	}
	return err
}

// OK implements Conn.OK by sending a successful JSON message inclding the
// given formatted text.
func (conn *connection) OK(format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)
	if err := writeResponse(conn, apiparams.OK, msg); err != nil {
		return errgo.Notef(err, "original message: %s", msg)
	}
	return nil
}

func writeResponse(conn Conn, code apiparams.ResponseCode, message string) error {
	resp := apiparams.Response{
		Code:    code,
		Message: message,
	}
	log.Debugw("sending response", "code", code, "message", message)
	if err := conn.WriteJSON(resp); err != nil {
		return errgo.Notef(err, "cannot write WebSocket response")
	}
	return nil
}

// upgrader is an HTTP connection upgrader to WebSocket allowing all origins.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
	ReadBufferSize:  webSocketBufferSize,
	WriteBufferSize: webSocketBufferSize,
}

// webSocketBufferSize holds the frame size for WebSocket messages.
const webSocketBufferSize = 65536
