// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	"go.uber.org/zap/zapcore"

	"github.com/juju/jujushell/apiparams"
	"github.com/juju/jujushell/internal/api"
	"github.com/juju/jujushell/internal/juju"
	"github.com/juju/jujushell/internal/logging"
)

var serveWebSocketTests = []struct {
	about            string
	addrs            []string
	allowedUsers     []string
	authUser         string
	authErr          string
	ops              []apiparams.Operation
	expectedMessages []string
}{{
	about:            "invalid operation",
	addrs:            []string{"1.2.3.4"},
	ops:              []apiparams.Operation{"bad wolf"},
	expectedMessages: []string{`invalid operation "bad wolf": expected "login"`},
}, {
	about:            "no credentials provided",
	addrs:            []string{"1.2.3.4", "1.2.3.5:17070"},
	ops:              []apiparams.Operation{"login"},
	expectedMessages: []string{"cannot log into juju: either userpass or macaroons must be provided"},
}, {
	about:            "authentication error",
	addrs:            []string{"1.2.3.4"},
	authErr:          "bad wolf",
	ops:              []apiparams.Operation{"login"},
	expectedMessages: []string{"cannot log into juju: bad wolf"},
}, {
	about:            "user not allowed",
	addrs:            []string{"1.2.3.4"},
	allowedUsers:     []string{"who", "rose"},
	authUser:         "dalek",
	ops:              []apiparams.Operation{"login"},
	expectedMessages: []string{`user "dalek" is not allowed to access the service`},
}, {
	about:            "user allowed",
	addrs:            []string{"1.2.3.4"},
	allowedUsers:     []string{"who@external", "rose@external"},
	authUser:         "rose@external",
	ops:              []apiparams.Operation{"login", "bad wolf"},
	expectedMessages: []string{`logged in as "rose@external"`, `invalid operation "bad wolf": expected "start"`},
}, {
	about:            "everybody allowed",
	addrs:            []string{"1.2.3.4"},
	authUser:         "who",
	ops:              []apiparams.Operation{"login", "bad wolf"},
	expectedMessages: []string{`logged in as "who"`, `invalid operation "bad wolf": expected "start"`},
}}

func TestServeWebSocket(t *testing.T) {
	c := qt.New(t)
	logging.Log().SetLevel(zapcore.ErrorLevel)

	send := func(conn *websocket.Conn, op apiparams.Operation) string {
		err := conn.WriteJSON(apiparams.Login{
			Operation: op,
		})
		c.Assert(err, qt.Equals, nil)
		var resp apiparams.Response
		err = conn.ReadJSON(&resp)
		c.Assert(err, qt.Equals, nil)
		return resp.Message
	}

	for _, test := range serveWebSocketTests {
		c.Run(test.about, func(c *qt.C) {
			// Set up the WebSocket server.
			server := httptest.NewServer(setupMux(test.addrs, test.allowedUsers))
			defer server.Close()
			patchJujuAuthenticate(c, test.authUser, test.authErr, test.addrs)

			// Connect a WebSocket client to the server.
			conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
			c.Assert(err, qt.Equals, nil)
			defer conn.Close()

			// Run the operations.
			for i, op := range test.ops {
				msg := send(conn, op)
				c.Assert(msg, qt.Equals, test.expectedMessages[i], qt.Commentf("op %d", i))
			}
		})
	}
}

// setupMux creates and returns a mux with the API registered.
func setupMux(addrs, allowedUsers []string) *http.ServeMux {
	mux := http.NewServeMux()
	api.Register(mux, api.JujuParams{
		Addrs: addrs,
		Cert:  "cert",
	}, api.LXDParams{
		ImageName: "image",
		Profiles:  []string{"default", "termserver"},
	}, api.SvcParams{
		AllowedUsers: allowedUsers,
	})
	return mux
}

// wsURL returns a WebSocket URL from the given HTTP URL.
func wsURL(u string) string {
	return strings.Replace(u, "http://", "ws://", 1) + "/ws/"
}

func patchJujuAuthenticate(c *qt.C, user, err string, addrs []string) {
	c.Patch(api.JujuAuthenticate, func(addrs []string, creds *juju.Credentials, cert string) (*juju.Info, error) {
		c.Assert(addrs, qt.DeepEquals, addrs)
		c.Assert(cert, qt.Equals, "cert")
		if user != "" {
			return &juju.Info{
				User: user,
			}, nil
		}
		if err != "" {
			return nil, errors.New(err)
		}
		return juju.Authenticate(addrs, creds, cert)
	})
}
