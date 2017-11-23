// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wsproxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/CanonicalLtd/jujushell/internal/wsproxy"
)

func TestCopy(t *testing.T) {
	c := qt.New(t)
	// Set up a target WebSocket server.
	ping := httptest.NewServer(http.HandlerFunc(pingHandler))
	defer ping.Close()

	// Set up the WebSocket proxy that copies the messages back and forth.
	proxy := httptest.NewServer(newProxyHandler(wsURL(ping.URL)))
	defer proxy.Close()

	// Connect to the proxy.
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(proxy.URL), nil)
	c.Assert(err, qt.Equals, nil)

	// Send messages and check that ping responses are properly received.
	send := func(content string) string {
		msg := jsonMessage{
			Content: content,
		}
		err = conn.WriteJSON(msg)
		c.Assert(err, qt.Equals, nil)
		err = conn.ReadJSON(&msg)
		c.Assert(err, qt.Equals, nil)
		return msg.Content
	}
	c.Assert(send("ping"), qt.Equals, "ping pong")
	c.Assert(send("bad wolf"), qt.Equals, "bad wolf pong")
}

// pingHandler is a WebSocket handler responding to pings.
func pingHandler(w http.ResponseWriter, req *http.Request) {
	conn := upgrade(w, req)
	defer conn.Close()
	var msg jsonMessage
	for {
		err := conn.ReadJSON(&msg)
		if err == io.EOF {
			return
		}
		if err != nil {
			panic(err)
		}
		msg.Content += " pong"
		if err = conn.WriteJSON(msg); err != nil {
			panic(err)
		}
	}
}

// newCopyHandler returns a WebSocket handler copying from the given WebSocket
// server.
func newProxyHandler(srvURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn1 := upgrade(w, req)
		defer conn1.Close()
		conn2, _, err := websocket.DefaultDialer.Dial(srvURL, nil)
		if err != nil {
			panic(err)
		}
		defer conn2.Close()
		if err := wsproxy.Copy(conn1, conn2); err != nil {
			panic(err)
		}
	})
}

// logStorage is a logger.Interface used for testing purposes.
type logStorage struct {
	messages []string
}

// Print implements logger.Interface and stores log messages.
func (ls *logStorage) Print(msg string) {
	ls.messages = append(ls.messages, msg)
}

// wsURL returns a WebSocket URL from the given HTTP URL.
func wsURL(u string) string {
	return strings.Replace(u, "http://", "ws://", 1)
}

// upgrade upgrades the given request and returns the resulting WebSocket
// connection.
func upgrade(w http.ResponseWriter, req *http.Request) *websocket.Conn {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		panic(err)
	}
	return conn
}

// upgrader holds a zero valued WebSocket upgrader.
var upgrader = websocket.Upgrader{}

// jsonMessage holds messages used for testing the WebSocket handlers.
type jsonMessage struct {
	Content string
}
