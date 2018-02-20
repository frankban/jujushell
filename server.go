// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujushell

import (
	"net/http"
	"time"

	"gopkg.in/errgo.v1"

	"github.com/juju/jujushell/internal/api"
)

// NewServer returns a new handler that handles juju shell requests.
func NewServer(p Params) (http.Handler, error) {
	mux := http.NewServeMux()
	err := api.Register(mux, api.JujuParams{
		Addrs: p.JujuAddrs,
		Cert:  p.JujuCert,
	}, api.LXDParams{
		ImageName: p.ImageName,
		Profiles:  p.Profiles,
	}, api.SvcParams{
		AllowedUsers:    p.AllowedUsers,
		SessionDuration: p.SessionDuration,
		WelcomeMessage:  p.WelcomeMessage,
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return mux, nil
}

// Params holds parameters for running the server.
type Params struct {
	// AllowedUsers holds a list of names of users allowed to use the service.
	AllowedUsers []string
	// ImageName holds the name of the LXD image to use to create containers.
	ImageName string
	// JujuAddrs holds the addresses of the current Juju controller.
	JujuAddrs []string
	// JujuCert holds the controller CA certificate in PEM format.
	JujuCert string
	// Profiles holds the LXD profiles to use when launching containers.
	Profiles []string
	// SessionDuration holds time duration before expiring container sessions.
	SessionDuration time.Duration
	// WelcomeMessage optionally holds an initial welcome message for users.
	WelcomeMessage string
}
