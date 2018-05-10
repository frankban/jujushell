// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"errors"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/api"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/network"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	macaroon "gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/jujushell/internal/juju"
)

var (
	addrs = []string{"1.2.3.4"}
	cert  = "juju-cert"
)

var authenticateTests = []struct {
	about                 string
	username              string
	password              string
	macaroons             map[string]macaroon.Slice
	apiOpenUsername       string
	apiOpenControllerUUID string
	apiOpenEndpoints      []string
	apiOpenError          string
	expectedInfo          *juju.Info
	expectedError         string
	expectedClosed        bool
}{{
	about:                 "userpass authentication",
	username:              "who",
	password:              "tardis",
	apiOpenUsername:       "rose",
	apiOpenControllerUUID: "c1-uuid",
	apiOpenEndpoints:      []string{"1.2.3.4:42", "1.2.3.4:47"},
	expectedInfo: &juju.Info{
		User:           "rose",
		ControllerName: "ctrl",
		ControllerUUID: "c1-uuid",
		CACert:         cert,
		Endpoints:      []string{"1.2.3.4:42", "1.2.3.4:47"},
	},
	expectedClosed: true,
}, {
	about: "macaroon authentication",
	macaroons: map[string]macaroon.Slice{
		"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
	},
	apiOpenUsername:       "rose",
	apiOpenControllerUUID: "c2-uuid",
	apiOpenEndpoints:      []string{"1.2.3.4:42"},
	expectedInfo: &juju.Info{
		User:           "rose",
		ControllerName: "ctrl",
		ControllerUUID: "c2-uuid",
		CACert:         cert,
		Endpoints:      []string{"1.2.3.4:42"},
	},
	expectedClosed: true,
}, {
	about:         "no credentials provided",
	expectedError: "either userpass or macaroons must be provided",
}, {
	about: "bad macaroons",
	macaroons: map[string]macaroon.Slice{
		":::": macaroon.Slice{mustNewMacaroon("m1")},
	},
	expectedError: "cannot store macaroons for logging into controller: cannot parse macaroon URL .*",
}, {
	about:         "authentication error",
	username:      "who",
	password:      "tardis",
	apiOpenError:  "bad wolf",
	expectedError: "cannot authenticate user: bad wolf",
}}

func TestAuthenticate(t *testing.T) {
	c := qt.New(t)
	for _, test := range authenticateTests {
		c.Run(test.about, func(c *qt.C) {
			conn := &connection{
				username:       test.apiOpenUsername,
				controllerUUID: test.apiOpenControllerUUID,
				endpoints:      test.apiOpenEndpoints,
			}
			var apiOpenError error
			if test.apiOpenError != "" {
				apiOpenError = errors.New(test.apiOpenError)
			} else {
				apiOpenError = nil
			}
			expectedInfo := &api.Info{
				Password: test.password,
			}
			if test.username != "" {
				expectedInfo.Tag = names.NewUserTag(test.username)
			}
			patchAPIOpen(c, conn, apiOpenError, expectedInfo, test.macaroons)
			info, err := juju.Authenticate(addrs, &juju.Credentials{
				Username:  test.username,
				Password:  test.password,
				Macaroons: test.macaroons,
			}, cert)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(info, qt.IsNil)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(info, qt.DeepEquals, test.expectedInfo)
			}
			c.Assert(conn.closed, qt.Equals, test.expectedClosed)
		})
	}
}

var setMacaroonsTests = []struct {
	about         string
	macaroons     map[string]macaroon.Slice
	expectedError string
}{{
	about: "success",
	macaroons: map[string]macaroon.Slice{
		"https://1.2.3.4/": macaroon.Slice{mustNewMacaroon("m1-test")},
		"https://4.3.2.1/": macaroon.Slice{mustNewMacaroon("m2-test")},
	},
}, {
	about: "error: bad url",
	macaroons: map[string]macaroon.Slice{
		"https://1.2.3.4/": macaroon.Slice{mustNewMacaroon("m1-test")},
		":::":              macaroon.Slice{mustNewMacaroon("m2-test")},
	},
	expectedError: `cannot parse macaroon URL ":::": .*`,
}, {
	about: "error: bad url",
	macaroons: map[string]macaroon.Slice{
		"https://1.2.3.4/": macaroon.Slice{},
	},
	expectedError: `cannot create cookie for "https://1.2.3.4/": no macaroons in cookie`,
}}

func TestSetMacaroons(t *testing.T) {
	for _, test := range setMacaroonsTests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)
			// Set up the cookie jar.
			jar, err := cookiejar.New(nil)
			c.Assert(err, qt.Equals, nil)
			err = juju.SetMacaroons(jar, test.macaroons)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				return
			}
			// The macaroons have been stored in the jar.
			c.Assert(err, qt.Equals, nil)
			for uStr, ms := range test.macaroons {
				u := mustParseURL(uStr)
				cookies := jar.Cookies(u)
				expectedCookie, err := httpbakery.NewCookie(ms)
				c.Assert(err, qt.Equals, nil)
				c.Assert(cookies[0], qt.DeepEquals, expectedCookie)
			}
		})
	}
}

func TestMarshalAccounts(t *testing.T) {
	c := qt.New(t)
	expectedAccounts := map[string]map[string]jujuclient.AccountDetails{
		"controllers": {
			"my-controller": jujuclient.AccountDetails{
				User:     "who",
				Password: "secret!",
			},
		},
	}
	data, err := juju.MarshalAccounts("my-controller", "who", "secret!")
	c.Assert(err, qt.Equals, nil)
	var accounts map[string]map[string]jujuclient.AccountDetails
	err = yaml.Unmarshal(data, &accounts)
	c.Assert(err, qt.Equals, nil)
	c.Assert(accounts, qt.DeepEquals, expectedAccounts)
}

func TestMarshalControllers(t *testing.T) {
	c := qt.New(t)
	info := &juju.Info{
		User:           "rose",
		ControllerName: "ctrl",
		ControllerUUID: "c1-uuid",
		CACert:         cert,
		Endpoints:      []string{"1.2.3.4:42", "1.2.3.4:47"},
	}
	expectedControllers := jujuclient.Controllers{
		Controllers: map[string]jujuclient.ControllerDetails{
			"ctrl": {
				ControllerUUID: "c1-uuid",
				APIEndpoints:   []string{"1.2.3.4:42", "1.2.3.4:47"},
				CACert:         "juju-cert",
			},
		},
		CurrentController: "ctrl",
	}
	data, err := juju.MarshalControllers(info)
	c.Assert(err, qt.Equals, nil)
	var controllers jujuclient.Controllers
	err = yaml.Unmarshal(data, &controllers)
	c.Assert(err, qt.Equals, nil)
	c.Assert(controllers, qt.DeepEquals, expectedControllers)
}

// patchAPIOpen patches the juju.apiOpen variable so that it is possible
// to simulate different API connection scenarios.
func patchAPIOpen(c *qt.C, conn api.Connection, err error, expectedInfo *api.Info, expectedMacaroons map[string]macaroon.Slice) {
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		c.Assert(info.Addrs, qt.DeepEquals, addrs)
		c.Assert(info.CACert, qt.Equals, cert)
		if info.Tag != nil {
			c.Assert(info.Tag.String(), qt.Equals, expectedInfo.Tag.String())
		}
		c.Assert(info.Password, qt.Equals, expectedInfo.Password)
		c.Assert(info.Macaroons, qt.IsNil)
		c.Assert(opts.RetryDelay, qt.Equals, 500*time.Millisecond)
		c.Assert(opts.Timeout, qt.Equals, 15*time.Second)
		for u, ms := range expectedMacaroons {
			cookies := opts.BakeryClient.Jar.Cookies(mustParseURL(u))
			expectedCookie, err := httpbakery.NewCookie(ms)
			c.Assert(err, qt.Equals, nil)
			c.Assert(cookies[0], qt.DeepEquals, expectedCookie)
		}
		return conn, err
	}
	c.Patch(juju.APIOpen, apiOpen)
}

type connection struct {
	api.Connection
	username       string
	controllerUUID string
	endpoints      []string
	closed         bool
}

// AuthTag implements api.Connection by returning a tag for the stored
// username.
func (c *connection) AuthTag() names.Tag {
	return names.NewUserTag(c.username)
}

// ControllerTag implements api.Connection by returning a tag for the stored
// controller unique identifier.
func (c *connection) ControllerTag() names.ControllerTag {
	return names.NewControllerTag(c.controllerUUID)
}

// APIHostPorts implements api.Connection by returning the stored hosts.
func (c *connection) APIHostPorts() [][]network.HostPort {
	hps, err := network.ParseHostPorts(c.endpoints...)
	if err != nil {
		panic(err)
	}
	return [][]network.HostPort{hps}
}

// Close implements api.Connection by setting this connection as closed.
func (c *connection) Close() error {
	c.closed = true
	return nil
}

func mustNewMacaroon(root string) *macaroon.Macaroon {
	m, err := macaroon.New([]byte(root), "id", "loc")
	if err != nil {
		panic(err)
	}
	return m
}

func mustParseURL(uStr string) *url.URL {
	u, err := url.Parse(uStr)
	if err != nil {
		panic(err)
	}
	return u
}
