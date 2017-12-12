// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujushell

import (
	"net/http"

	"github.com/juju/jujushell/internal/api"
)

// NewServer returns a new handler that handles juju shell requests.
func NewServer(p Params) (http.Handler, error) {
	mux := http.NewServeMux()
	api.Register(mux, api.JujuParams{
		Addrs: p.JujuAddrs,
		Cert:  p.JujuCert,
	}, api.LXDParams{
		ImageName: p.ImageName,
		Profiles:  p.Profiles,
	})
	return mux, nil
}

// Params holds parameters for running the server.
type Params struct {
	// ImageName holds the name of the LXD image to use to create containers.
	ImageName string
	// JujuAddrs holds the addresses of the current Juju controller.
	JujuAddrs []string
	// JujuCert holds the controller CA certificate in PEM format.
	JujuCert string
	// Profiles holds the LXD profiles to use when launching containers.
	Profiles []string `yaml:"profiles"`
}
