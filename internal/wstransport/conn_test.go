// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wstransport_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/juju/jujushell/apiparams"
	"github.com/juju/jujushell/internal/wstransport"
)

func TestConnError(t *testing.T) {
	c := qt.New(t)

	// Set up a WebSocket server that writes a JSON error response.
	srv := httptest.NewServer(wsHandler(func(conn wstransport.Conn) {
		badWolf := errors.New("bad wolf")
		err := conn.Error(badWolf)
		c.Assert(err, qt.Equals, badWolf)
	}))
	defer srv.Close()

	// Connect to the server.
	conn := dial(c, srv.URL)
	defer conn.Close()

	// Check tehe message from the server.
	var resp apiparams.Response
	err := conn.ReadJSON(&resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp, qt.DeepEquals, apiparams.Response{
		Code:    apiparams.Error,
		Message: "bad wolf",
	})
}

func TestConnOK(t *testing.T) {
	c := qt.New(t)

	// Set up a WebSocket server that writes a JSON successful response.
	srv := httptest.NewServer(wsHandler(func(conn wstransport.Conn) {
		err := conn.OK("these %s the voyages", "are")
		c.Assert(err, qt.Equals, nil)
	}))
	defer srv.Close()

	// Connect to the server.
	conn := dial(c, srv.URL)
	defer conn.Close()

	// Check tehe message from the server.
	var resp apiparams.Response
	err := conn.ReadJSON(&resp)
	c.Assert(err, qt.Equals, nil)
	c.Assert(resp, qt.DeepEquals, apiparams.Response{
		Code:    apiparams.OK,
		Message: "these are the voyages",
	})
}

// wsURL returns a WebSocket URL from the given HTTP URL.
func wsURL(u string) string {
	return strings.Replace(u, "http://", "ws://", 1)
}

// wsHanlder returns an http.Handler upgrading the connection to WebSocket and
// calling the given function with the resultig WebSocket connection.
func wsHandler(f func(conn wstransport.Conn)) http.Handler {
	handler := func(w http.ResponseWriter, req *http.Request) {
		conn := upgrade(w, req)
		defer conn.Close()
		f(conn)
	}
	return http.HandlerFunc(handler)
}

// upgrade upgrades the given HTTP request and returns the resulting WebSocket
// connection.
func upgrade(w http.ResponseWriter, req *http.Request) wstransport.Conn {
	conn, err := wstransport.Upgrade(w, req)
	if err != nil {
		panic(err)
	}
	return conn
}

// dial opens a WebSocket connection to the server at the given URL.
func dial(c *qt.C, url string) *websocket.Conn {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(url), nil)
	c.Assert(err, qt.Equals, nil)
	return conn
}
