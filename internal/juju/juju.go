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
// addresses with the given credentials. It returns the authenticated user name
// or an error.
func Authenticate(addrs []string, creds *Credentials, cert string) (string, error) {
	info := &api.Info{
		Addrs:  addrs,
		CACert: cert,
	}
	if len(creds.Macaroons) != 0 {
		info.Macaroons = creds.Macaroons
	} else if creds.Username != "" && creds.Password != "" {
		info.Tag = names.NewUserTag(creds.Username)
		info.Password = creds.Password
	} else {
		return "", errgo.New("either userpass or macaroons must be provided")
	}
	opts := api.DialOpts{
		RetryDelay: 500 * time.Millisecond,
		Timeout:    15 * time.Second,
	}
	conn, err := apiOpen(info, opts)
	if err != nil {
		return "", errgo.Notef(err, "cannot authenticate user")
	}
	return conn.AuthTag().Id(), nil
}

// Credentials holds credentials for logging into a Juju controller.
type Credentials struct {
	Username  string
	Password  string
	Macaroons []macaroon.Slice
}

// apiOpen is defined as a variable for testing.
var apiOpen = func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}
