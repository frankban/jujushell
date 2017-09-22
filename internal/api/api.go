// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/CanonicalLtd/jujushell/apiparams"
	"github.com/CanonicalLtd/jujushell/internal/logging"
)

// Register registers the API handlers in the given mux.
func Register(mux *http.ServeMux, jujuAddrs []string) error {
	// TODO: validate jujuAddrs.
	mux.Handle("/ws/", serveWebsocket(jujuAddrs))
	return nil
}

// serveWebsocket handles WebSocket connections.
func serveWebsocket(jujuAddrs []string) http.Handler {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  webSocketBufferSize,
		WriteBufferSize: webSocketBufferSize,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logging.Logger()
		// Upgrade the HTTP connection.
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorw(
				"cannot upgrade to WebSocket",
				"url", r.URL,
				"err", err)
			return
		}
		log.Debugw("WebSocket connection established", "remote-addr", r.RemoteAddr)
		defer conn.Close()

		// Start serving WebSocket requests.
		var req apiparams.Request
		var resp apiparams.Response
		for {
			if err = conn.ReadJSON(&req); err != nil {
				log.Debugw("cannot read WebSocket request", "err", err)
				return
			}
			resp = handleRequest(req)
			if err = conn.WriteJSON(resp); err != nil {
				log.Debugw("cannot write WebSocket response", "err", err)
				return
			}
		}
		log.Debugw("WebSocket disconnected", "remote-addr", r.RemoteAddr)
	})
}

// handleRequest handles WebSocket request/response traffic.
func handleRequest(req apiparams.Request) apiparams.Response {
	switch req.Operation {
	case "ping":
		return apiparams.Response{
			Code:    apiparams.OK,
			Message: "pong",
		}
	}
	return apiparams.Response{
		Code:    apiparams.Error,
		Message: fmt.Sprintf("invalid operation %q", req.Operation),
	}
}

// webSocketBufferSize holds the frame size for WebSocket messages.
const webSocketBufferSize = 65536
