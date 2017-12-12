// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/juju/jujushell/apiparams"
)

// statusHandler is used to check whether the server is ready.
func statusHandler(w http.ResponseWriter, r *http.Request) {
	enc := json.NewEncoder(w)
	// Ignore errors here.
	enc.Encode(apiparams.Response{
		Code:    apiparams.OK,
		Message: "server is ready",
	})
}
