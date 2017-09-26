// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jujushell/apiparams"
	"github.com/CanonicalLtd/jujushell/internal/juju"
	"github.com/CanonicalLtd/jujushell/internal/logging"
	"github.com/CanonicalLtd/jujushell/internal/lxd"
	"github.com/CanonicalLtd/jujushell/internal/wsproxy"
)

// Register registers the API handlers in the given mux.
func Register(mux *http.ServeMux, jujuAddrs []string, imageName string) error {
	// TODO: validate jujuAddrs.
	mux.Handle("/ws/", serveWebSocket(jujuAddrs, imageName))
	return nil
}

// serveWebSocket handles WebSocket connections.
func serveWebSocket(jujuAddrs []string, imageName string) http.Handler {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  webSocketBufferSize,
		WriteBufferSize: webSocketBufferSize,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logging.Logger()
		// Upgrade the HTTP connection.
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorw("cannot upgrade to WebSocket", "url", r.URL, "err", err)
			return
		}
		log.Debugw("WebSocket connection established", "remote-addr", r.RemoteAddr)
		defer conn.Close()

		// Start serving requests.
		username, err := handleLogin(conn, jujuAddrs)
		if err != nil {
			log.Debugw("cannot authenticate the user", "err", err)
			return
		}
		address, err := handleStart(conn, username, imageName)
		if err != nil {
			log.Debugw("cannot start user session", "user", username, "err", err)
			return
		}
		if err = handleSession(conn, address); err != nil {
			log.Debugw("session closed", "user", username, "address", address, "err", err)
			return
		}
	})
}

// handleLogin checks that the user has the right credentials for logging into
// the Juju controller at the give addresses.
// Example request/response:
//     --> {"operation": "login", "username": "admin", "password": "secret"}
//     <-- {"code": "ok", "message": "logged in as \"admin\""}
func handleLogin(conn *websocket.Conn, jujuAddrs []string) (username string, err error) {
	var req apiparams.Login
	if err = conn.ReadJSON(&req); err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	if req.Operation != apiparams.OpLogin {
		return "", writeError(conn, errgo.Newf("invalid operation %q: expected %q", req.Operation, apiparams.OpLogin))
	}
	username, err = juju.Authenticate(jujuAddrs, req.Username, req.Password, req.Macaroons)
	if err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	return username, writeOK(conn, "logged in as %q", username)
}

// handleStart ensures an LXD is available for the given username, by checking
// whether one container is already started or, if not, creating one based on
// the provided image name. Example request/response:
//     --> {"operation": "start"}
//     <-- {"code": "ok", "message": "session is ready"}
func handleStart(conn *websocket.Conn, username, imageName string) (address string, err error) {
	var req apiparams.Start
	if err = conn.ReadJSON(&req); err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	if req.Operation != apiparams.OpStart {
		return "", writeError(conn, errgo.Newf("invalid operation %q: expected %q", req.Operation, apiparams.OpStart))
	}
	address, err = lxd.Ensure(username, imageName)
	if err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	return address, writeOK(conn, "session is ready")
}

// handleSession proxies traffic from the client to the LXD instance at the
// given address.
func handleSession(conn *websocket.Conn, address string) error {
	// The path must reflect what used by the Terminado service which is
	// running in the LXD container.
	addr := "ws://" + address + "/websocket"
	lxcconn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return errgo.Notef(err, "cannot dial %s", addr)
	}
	return errgo.Mask(wsproxy.Copy(conn, lxcconn))
}

func writeError(conn *websocket.Conn, err error) error {
	if werr := writeResponse(conn, apiparams.Error, err.Error()); werr != nil {
		return errgo.Notef(werr, "original error: %s", err)
	}
	return err
}

func writeOK(conn *websocket.Conn, format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)
	if werr := writeResponse(conn, apiparams.OK, msg); werr != nil {
		return errgo.Notef(werr, "original message: %s", msg)
	}
	return nil
}

func writeResponse(conn *websocket.Conn, code apiparams.ResponseCode, message string) error {
	resp := apiparams.Response{
		Code:    code,
		Message: message,
	}
	if err := conn.WriteJSON(resp); err != nil {
		return errgo.Notef(err, "cannot write WebSocket response")
	}
	return nil
}

// webSocketBufferSize holds the frame size for WebSocket messages.
const webSocketBufferSize = 65536
