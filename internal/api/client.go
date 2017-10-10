// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"encoding/json"
	"net/http"
	"time"

	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jujushell/apiparams"
)

const retries = 100

func waitReady(url string) error {
	var err error
	var resp *http.Response
	c := &http.Client{}
	for i := 0; i < retries; i++ {
		resp, err = c.Get(url)
		if err == nil {
			break
		}
		// Probably the server is just not running/listening yet.
		sleep(100 * time.Millisecond)
	}
	if err != nil {
		return errgo.Notef(err, "cannot get %s", url)
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	var r apiparams.Response
	if err = dec.Decode(&r); err != nil {
		return errgo.Notef(err, "cannot decode response")
	}
	if r.Code != apiparams.OK {
		return errgo.Newf("invalid response from %s: %q", url, r.Code)
	}
	return nil
}

// sleep is defined as a variable for testing purposes.
var sleep = func(d time.Duration) {
	time.Sleep(d)
}
