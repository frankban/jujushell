// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	"gopkg.in/juju/names.v2"
	macaroon "gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jujushell/internal/juju"
)

var authenticateTests = []struct {
	about            string
	username         string
	password         string
	macaroons        []macaroon.Slice
	apiOpenUsername  string
	apiOpenError     string
	expectedUsername string
	expectedError    string
}{{
	about:            "userpass authentication",
	username:         "who",
	password:         "tardis",
	apiOpenUsername:  "rose",
	expectedUsername: "rose",
}, {
	about:            "macaroon authentication",
	macaroons:        []macaroon.Slice{{mustNewMacaroon("m1")}},
	apiOpenUsername:  "rose",
	expectedUsername: "rose",
}, {
	about:         "no credentials provided",
	expectedError: "either userpass or macaroons must be provided",
}, {
	about:         "authentication error",
	username:      "who",
	password:      "tardis",
	apiOpenError:  "bad wolf",
	expectedError: "cannot authenticate user: bad wolf",
}}

func TestAuthenticate(t *testing.T) {
	for _, test := range authenticateTests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)
			conn := connection{
				username: test.apiOpenUsername,
			}
			var apiOpenError error
			if test.apiOpenError != "" {
				apiOpenError = errors.New(test.apiOpenError)
			} else {
				apiOpenError = nil
			}
			expectedInfo := &api.Info{
				Password:  test.password,
				Macaroons: test.macaroons,
			}
			if test.username != "" {
				expectedInfo.Tag = names.NewUserTag(test.username)
			}
			restore := patchAPIOpen(c, conn, apiOpenError, expectedInfo)
			defer restore()
			username, err := juju.Authenticate(
				[]string{"1.2.3.4"}, test.username, test.password, test.macaroons)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(username, qt.Equals, "")
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(username, qt.Equals, test.expectedUsername)
			}
		})
	}
}

// patchAPIOpen patches the juju.apiOpen variable so that it is possible
// to simulate different API connection scenarios.
func patchAPIOpen(c *qt.C, conn api.Connection, err error, expectedInfo *api.Info) (restore func()) {
	original := *juju.APIOpen
	*juju.APIOpen = func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Assert(info.Addrs, qt.DeepEquals, []string{"1.2.3.4"})
		if info.Tag != nil {
			c.Assert(info.Tag.String(), qt.Equals, expectedInfo.Tag.String())
		}
		c.Assert(info.Password, qt.Equals, expectedInfo.Password)
		if expectedInfo.Macaroons != nil {
			c.Assert(len(info.Macaroons), qt.Equals, len(expectedInfo.Macaroons))
			c.Assert(info.Macaroons[0][0].Signature(), qt.DeepEquals, expectedInfo.Macaroons[0][0].Signature())
		} else {
			c.Assert(info.Macaroons, qt.IsNil)
		}
		c.Assert(opts, qt.DeepEquals, api.DialOpts{
			InsecureSkipVerify: true,
			RetryDelay:         500 * time.Millisecond,
			Timeout:            15 * time.Second,
		})
		return conn, err
	}
	return func() {
		*juju.APIOpen = original
	}
}

type connection struct {
	api.Connection
	username string
}

// AuthTag implements api.Connection by returning a tag for the stored
// username.
func (c connection) AuthTag() names.Tag {
	return names.NewUserTag(c.username)
}

func mustNewMacaroon(root string) *macaroon.Macaroon {
	m, err := macaroon.New([]byte(root), "id", "loc")
	if err != nil {
		panic(err)
	}
	return m
}
