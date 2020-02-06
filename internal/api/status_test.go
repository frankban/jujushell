// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/juju/jujushell/apiparams"
)

func TestStatusHandler(t *testing.T) {
	c := qt.New(t)
	defer c.Done()
	// Set up the WebSocket server.
	server := httptest.NewServer(setupMux(c, []string{"1.2.3.4"}, nil))
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
	c.Assert(r.Operation, qt.Equals, apiparams.OpStatus)
	c.Assert(r.Code, qt.Equals, apiparams.OK)
	c.Assert(r.Message, qt.Equals, "server is ready")
}
