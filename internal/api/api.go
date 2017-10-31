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
	"github.com/CanonicalLtd/jujushell/internal/lxdutils"
	"github.com/CanonicalLtd/jujushell/internal/wsproxy"
)

// Register registers the API handlers in the given mux.
func Register(mux *http.ServeMux, jujuAddrs []string, jujuCert, image string) error {
	// TODO: validate jujuAddrs.
	mux.Handle("/ws/", serveWebSocket(jujuAddrs, jujuCert, image))
	mux.HandleFunc("/status/", statusHandler)
	return nil
}

// serveWebSocket handles WebSocket connections.
func serveWebSocket(jujuAddrs []string, jujuCert, image string) http.Handler {
	upgrader := websocket.Upgrader{
		// TODO: only allow request from the controller addresses.
		CheckOrigin: func(*http.Request) bool {
			return true
		},
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
		log.Infow("WebSocket connection established", "remote-addr", r.RemoteAddr)
		defer conn.Close()

		// Start serving requests.
		info, creds, err := handleLogin(conn, jujuAddrs, jujuCert)
		if err != nil {
			log.Debugw("cannot authenticate the user", "err", err)
			return
		}
		log.Debugw("user authenticated", "user", info.User, "uuid", info.ControllerUUID, "endpoints", info.Endpoints)
		addr, err := handleStart(conn, image, info, creds)
		if err != nil {
			log.Debugw("cannot start user session", "user", info.User, "err", err)
			return
		}
		log.Debugw("session started", "user", info.User, "address", addr)
		if err = handleSession(conn, addr); err != nil {
			log.Debugw("session closed", "user", info.User, "address", addr, "err", err)
			return
		}
		log.Infow("closing WebSocket connection", "remote-addr", r.RemoteAddr)
	})
}

// handleLogin checks that the user has the right credentials for logging into
// the Juju controller at the give addresses.
// Example request/response:
//     --> {"operation": "login", "username": "admin", "password": "secret"}
//     <-- {"code": "ok", "message": "logged in as \"admin\""}
func handleLogin(conn *websocket.Conn, jujuAddrs []string, jujuCert string) (info *juju.Info, creds *juju.Credentials, err error) {
	var req apiparams.Login
	if err = conn.ReadJSON(&req); err != nil {
		return nil, nil, writeError(conn, errgo.Mask(err))
	}
	if req.Operation != apiparams.OpLogin {
		return nil, nil, writeError(conn, errgo.Newf("invalid operation %q: expected %q", req.Operation, apiparams.OpLogin))
	}
	creds = &juju.Credentials{
		Username:  req.Username,
		Password:  req.Password,
		Macaroons: req.Macaroons,
	}
	info, err = juju.Authenticate(jujuAddrs, creds, jujuCert)
	if err != nil {
		return nil, nil, writeError(conn, errgo.Notef(err, "cannot log into juju"))
	}
	return info, creds, writeOK(conn, "logged in as %q", info.User)
}

// handleStart ensures an LXD is available for the given username, by checking
// whether one container is already started or, if not, creating one based on
// the provided image name. Example request/response:
//     --> {"operation": "start"}
//     <-- {"code": "ok", "message": "session is ready"}
func handleStart(conn *websocket.Conn, image string, info *juju.Info, creds *juju.Credentials) (addr string, err error) {
	var req apiparams.Start
	if err = conn.ReadJSON(&req); err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	if req.Operation != apiparams.OpStart {
		return "", writeError(conn, errgo.Newf("invalid operation %q: expected %q", req.Operation, apiparams.OpStart))
	}
	lxdsrv, err := lxdutils.Connect()
	if err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	addr, err = lxdutils.Ensure(lxdsrv, image, info, creds)
	if err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	url := fmt.Sprintf("http://%s:%d/status", addr, termserverPort)
	if err = waitReady(url); err != nil {
		return "", writeError(conn, errgo.Mask(err))
	}
	return addr, writeOK(conn, "session is ready")
}

// handleSession proxies traffic from the client to the LXD instance at the
// given address.
func handleSession(conn *websocket.Conn, addr string) error {
	// The path must reflect what used by the Terminado service which is
	// running in the LXD container.
	url := fmt.Sprintf("ws://%s:%d/websocket", addr, termserverPort)
	lxcconn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return errgo.Notef(err, "cannot dial %s", url)
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

const (
	// webSocketBufferSize holds the frame size for WebSocket messages.
	webSocketBufferSize = 65536
	// termserverPort holds the port on which the term server is listening.
	termserverPort = 8765
)
