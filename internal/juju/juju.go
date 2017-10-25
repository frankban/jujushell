// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"net/http"
	"net/url"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
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
	var client *httpbakery.Client
	if len(creds.Macaroons) != 0 {
		client = httpbakery.NewClient()
		if err := SetMacaroons(client.Jar, creds.Macaroons); err != nil {
			return "", errgo.Notef(err, "cannot store macaroons for logging into controller")
		}
	} else if creds.Username != "" && creds.Password != "" {
		info.Tag = names.NewUserTag(creds.Username)
		info.Password = creds.Password
	} else {
		return "", errgo.New("either userpass or macaroons must be provided")
	}
	opts := api.DialOpts{
		RetryDelay:   500 * time.Millisecond,
		Timeout:      15 * time.Second,
		BakeryClient: client,
	}
	conn, err := apiOpen(info, opts)
	if err != nil {
		return "", errgo.Notef(err, "cannot authenticate user")
	}
	return conn.AuthTag().Id(), nil
}

// Credentials holds credentials for logging into a Juju controller.
type Credentials struct {
	// Username and Password hold traditional Juju credentials for local users.
	Username string
	Password string
	// Macaroons, alternatively, maps cookie URLs to macaroons used for
	// authenticating as external users. An identity manager URL/token pair is
	// usually provided.
	Macaroons map[string]macaroon.Slice
}

// SetMacaroons sets the given macaroons as cookies in the given jar.
func SetMacaroons(jar http.CookieJar, macaroons map[string]macaroon.Slice) error {
	for uStr, ms := range macaroons {
		u, err := url.Parse(uStr)
		if err != nil {
			return errgo.Notef(err, "cannot parse macaroon URL %q", uStr)
		}
		cookie, err := httpbakery.NewCookie(ms)
		if err != nil {
			return errgo.Notef(err, "cannot create cookie for %q", uStr)
		}
		jar.SetCookies(u, []*http.Cookie{cookie})
	}
	return nil
}

// apiOpen is defined as a variable for testing.
var apiOpen = func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}
