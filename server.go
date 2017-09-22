// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujushell

import (
	"net/http"

	"github.com/CanonicalLtd/jujushell/internal/api"
)

// NewServer returns a new handler that handles juju shell requests.
func NewServer(p Params) (http.Handler, error) {
	mux := http.NewServeMux()
	if err := api.Register(mux, p.JujuAddrs); err != nil {
		return nil, err
	}
	return mux, nil
}

// Params holds parameters for running the server.
type Params struct {
	// JujuAddrs holds the addresses of the current Juju controller.
	JujuAddrs []string
}
