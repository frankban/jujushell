// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jujushell/apiparams"
	"github.com/CanonicalLtd/jujushell/internal/api"
)

func TestStatusHandler(t *testing.T) {
	c := qt.New(t)
	// Set up the WebSocket server.
	mux := http.NewServeMux()
	jujuAddrs, jujuCert, imageName := []string{"1.2.3.4"}, "cert", "image"
	err := api.Register(mux, jujuAddrs, jujuCert, imageName)
	c.Assert(err, qt.Equals, nil)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Exercise the status handler.
	client := &http.Client{}
	resp, err := client.Get(server.URL + "/status/")
	c.Assert(err, qt.Equals, nil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, qt.Equals, http.StatusOK)
	dec := json.NewDecoder(resp.Body)
	var r apiparams.Response
	err = dec.Decode(&r)
	c.Assert(err, qt.Equals, nil)
	c.Assert(r.Code, qt.Equals, apiparams.OK)
	c.Assert(r.Message, qt.Equals, "server is ready")
}
