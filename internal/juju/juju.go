// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	macaroon "gopkg.in/macaroon.v1"
)

// Authenticate logs the current user into the Juju controller at the given
// addresses with the given userpass pair or macaroons. It returns the
// authenticated user name or an error.
func Authenticate(addrs []string, username, password string, macaroons []macaroon.Slice) (string, error) {
	info := &api.Info{
		Addrs: addrs,
		// TODO: CACert:   ...,
	}
	if len(macaroons) != 0 {
		info.Macaroons = macaroons
	} else if username != "" && password != "" {
		info.Tag = names.NewUserTag(username)
		info.Password = password
	} else {
		return "", errgo.New("either userpass or macaroons must be provided")
	}
	opts := api.DialOpts{
		InsecureSkipVerify: true, // TODO: remove this.
		RetryDelay:         500 * time.Millisecond,
		Timeout:            15 * time.Second,
	}
	conn, err := apiOpen(info, opts)
	if err != nil {
		return "", errgo.Notef(err, "cannot authenticate user")
	}
	return conn.AuthTag().Id(), nil
}

// apiOpen is defined as a variable for testing.
var apiOpen = func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}
