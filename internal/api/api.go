// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/errgo.v1"

	"github.com/juju/jujushell/apiparams"
	"github.com/juju/jujushell/internal/juju"
	"github.com/juju/jujushell/internal/logging"
	"github.com/juju/jujushell/internal/lxdutils"
	"github.com/juju/jujushell/internal/metrics"
	"github.com/juju/jujushell/internal/registry"
	"github.com/juju/jujushell/internal/wsproxy"
	"github.com/juju/jujushell/internal/wstransport"
)

var log = logging.Log()

// Register registers the API handlers in the given mux.
func Register(mux *http.ServeMux, juju JujuParams, lxd LXDParams, svc SvcParams) error {
	reg, err := registryNew(svc.SessionDuration)
	if err != nil {
		return errgo.Notef(err, "cannot create container registry")
	}
	mux.Handle("/ws/", metrics.InstrumentHandler(serveWebSocket(juju, lxd, svc, reg)))
	mux.HandleFunc("/status/", statusHandler)
	mux.Handle("/metrics", promhttp.Handler())
	return nil
}

// JujuParams holds parameters for interacting with the Juju controller.
type JujuParams struct {
	// Addrs holds the addresses of the current Juju controller.
	Addrs []string
	// Cert holds the controller CA certificate in PEM format.
	Cert string
}

// LXDParams holds parameters used for creating LXD containers.
type LXDParams struct {
	// ImageName holds the name of the LXD image to use.
	ImageName string
	// Profiles holds the LXD profile names.
	Profiles []string `yaml:"profiles"`
}

// SvcParams holds parameters used for configuring and running the service.
type SvcParams struct {
	// AllowedUsers holds a list of names of users allowed to use the service.
	AllowedUsers []string
	// SessionDuration holds time duration before expiring container sessions.
	SessionDuration time.Duration
}

// serveWebSocket handles WebSocket connections.
func serveWebSocket(juju JujuParams, lxd LXDParams, svc SvcParams, reg *registry.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Upgrade the HTTP connection.
		conn, err := wstransport.Upgrade(w, r)
		if err != nil {
			log.Errorw("cannot upgrade to WebSocket", "url", r.URL, "err", err)
			return
		}
		defer conn.Close()
		conn = metrics.InstrumentWSConnection(conn)
		log.Infow("WebSocket connection established", "remote-addr", r.RemoteAddr)

		// Start serving requests.
		info, creds, err := handleLogin(conn, juju.Addrs, juju.Cert, svc.AllowedUsers)
		if err != nil {
			log.Infow("cannot authenticate the user", "err", err)
			return
		}
		log.Infow("user authenticated", "user", info.User, "uuid", info.ControllerUUID, "endpoints", info.Endpoints)
		name, addr, err := handleStart(conn, lxd, info, creds)
		if err != nil {
			log.Infow("cannot start user session", "user", info.User, "err", err)
			return
		}
		log.Infow("session started", "user", info.User, "address", addr)
		if err = handleSession(conn, name, addr, reg); err != nil {
			log.Infow("session closed", "user", info.User, "address", addr, "err", err)
			return
		}
		log.Infow("closing WebSocket connection", "remote-addr", r.RemoteAddr)
	})
}

// handleLogin checks that the user has the right credentials for logging into
// the Juju controller at the given addresses. If the provided list of allowed
// users is not empty, this function also checks that the user is allowed.
// Example request/response:
//     --> {"operation": "login", "username": "admin", "password": "secret"}
//     <-- {"code": "ok", "message": "logged in as \"admin\""}
func handleLogin(conn wstransport.Conn, jujuAddrs []string, jujuCert string, allowedUsers []string) (info *juju.Info, creds *juju.Credentials, err error) {
	var req apiparams.Login
	if err = conn.ReadJSON(&req); err != nil {
		return nil, nil, conn.Error(errgo.Mask(err))
	}
	if req.Operation != apiparams.OpLogin {
		return nil, nil, conn.Error(errgo.Newf("invalid operation %q: expected %q", req.Operation, apiparams.OpLogin))
	}
	creds = &juju.Credentials{
		Username:  req.Username,
		Password:  req.Password,
		Macaroons: req.Macaroons,
	}
	log.Debugw("authenticating to the controller", "addresses", jujuAddrs)
	info, err = jujuAuthenticate(jujuAddrs, creds, jujuCert)
	if err != nil {
		return nil, nil, conn.Error(errgo.Notef(err, "cannot log into juju"))
	}
	if !isUserAllowed(info.User, allowedUsers) {
		return nil, nil, conn.Error(errgo.Newf("user %q is not allowed to access the service", info.User))
	}
	return info, creds, conn.OK("logged in as %q", info.User)
}

// handleStart ensures an LXD is available for the given username, by checking
// whether one container is already started or, if not, creating one based on
// the provided LXD parameters. Example request/response:
//     --> {"operation": "start"}
//     <-- {"code": "ok", "message": "session is ready"}
func handleStart(conn wstransport.Conn, lxd LXDParams, info *juju.Info, creds *juju.Credentials) (name, addr string, err error) {
	var req apiparams.Start
	if err = conn.ReadJSON(&req); err != nil {
		return "", "", conn.Error(errgo.Mask(err))
	}
	if req.Operation != apiparams.OpStart {
		return "", "", conn.Error(errgo.Newf("invalid operation %q: expected %q", req.Operation, apiparams.OpStart))
	}
	log.Debugw("connecting to the LXD server")
	lxdclient, err := lxdutils.Connect()
	if err != nil {
		return "", "", conn.Error(errgo.Mask(err))
	}
	lxdclient = metrics.InstrumentLXDClient(lxdclient)
	log.Debugw("setting up the LXD instance", "image", lxd.ImageName, "profiles", lxd.Profiles)
	name, addr, err = lxdutils.Ensure(lxdclient, lxd.ImageName, lxd.Profiles, info, creds)
	if err != nil {
		return "", "", conn.Error(errgo.Mask(err))
	}
	url := fmt.Sprintf("http://%s:%d/status", addr, termserverPort)
	log.Debugw("waiting for the internal shell service to be ready", "url", url)
	if err = waitReady(url); err != nil {
		return "", "", conn.Error(errgo.Mask(err))
	}
	return name, addr, conn.OK("session is ready")
}

// handleSession proxies traffic from the client to the LXD instance with the
// given name and address.
func handleSession(conn wstransport.Conn, name, addr string, reg *registry.Registry) error {
	ac := reg.Get(name)
	ac.SetActive()
	// The path must reflect what used by the Terminado service which is
	// running in the LXD container.
	url := fmt.Sprintf("ws://%s:%d/websocket", addr, termserverPort)
	log.Debugw("connecting to internal shell service", "url", url)
	lxcconn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return errgo.Notef(err, "cannot dial %s", url)
	}
	defer lxcconn.Close()

	log.Debugw("starting the proxy")
	if err = wsproxy.Copy(wsproxy.NewConnWithHooks(conn, ac.SetActive), lxcconn); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// termserverPort holds the port on which the term server is listening.
const termserverPort = 8765

// isUserAllowed reports whether the provided user is allowed to access the
// service.
func isUserAllowed(user string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, u := range allowed {
		if u == user {
			return true
		}
	}
	return false
}

// jujuAuthenticate is defined as a variable for testing.
var jujuAuthenticate = func(addrs []string, creds *juju.Credentials, cert string) (*juju.Info, error) {
	return juju.Authenticate(addrs, creds, cert)
}

// registryNew is defined as a variable for testing.
var registryNew = func(d time.Duration) (*registry.Registry, error) {
	return registry.New(d)
}
