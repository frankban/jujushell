// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"

	"github.com/CanonicalLtd/jujushell/apiparams"
	"github.com/CanonicalLtd/jujushell/internal/api"
)

var registerTests = []struct {
	op            apiparams.Operation
	expectedError string
}{{
	op:            "bad wolf",
	expectedError: `invalid operation "bad wolf": expected "login"`,
}, {
	op:            "login",
	expectedError: `cannot log into juju: either userpass or macaroons must be provided`,
}}

func TestRegister(t *testing.T) {
	c := qt.New(t)
	// Set up the WebSocket server.
	mux := http.NewServeMux()
	jujuAddrs, jujuCert, imageName := []string{"1.2.3.4"}, "cert", "image"
	err := api.Register(mux, jujuAddrs, jujuCert, imageName)
	c.Assert(err, qt.Equals, nil)
	server := httptest.NewServer(mux)
	defer server.Close()

	send := func(op apiparams.Operation) string {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
		c.Assert(err, qt.Equals, nil)
		err = conn.WriteJSON(apiparams.Login{
			Operation: op,
		})
		c.Assert(err, qt.Equals, nil)
		var resp apiparams.Response
		err = conn.ReadJSON(&resp)
		c.Assert(err, qt.Equals, nil)
		return resp.Message
	}

	for _, test := range registerTests {
		c.Run(string(test.op), func(c *qt.C) {
			c.Assert(send(test.op), qt.Equals, test.expectedError)
		})
	}
}

// wsURL returns a WebSocket URL from the given HTTP URL.
func wsURL(u string) string {
	return strings.Replace(u, "http://", "ws://", 1) + "/ws/"
}
