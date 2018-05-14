// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"net/http"
	"net/url"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/network"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
	httpbakeryV1 "gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	macaroon "gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"
)

// Authenticate logs the current user into the Juju controller at the given
// addresses with the given credentials. It returns information about the Juju
// controller or an error.
func Authenticate(addrs []string, creds *Credentials, cert string) (*Info, error) {
	info := &api.Info{
		Addrs:  addrs,
		CACert: cert,
	}
	var client *httpbakeryV1.Client
	if len(creds.Macaroons) != 0 {
		client = httpbakeryV1.NewClient()
		if err := SetMacaroons(client.Jar, creds.Macaroons); err != nil {
			return nil, errgo.Notef(err, "cannot store macaroons for logging into controller")
		}
	} else if creds.Username != "" && creds.Password != "" {
		info.Tag = names.NewUserTag(creds.Username)
		info.Password = creds.Password
	} else {
		return nil, errgo.New("either userpass or macaroons must be provided")
	}
	opts := api.DialOpts{
		RetryDelay:   500 * time.Millisecond,
		Timeout:      15 * time.Second,
		BakeryClient: client,
	}
	conn, err := apiOpen(info, opts)
	if err != nil {
		return nil, errgo.Notef(err, "cannot authenticate user")
	}
	defer conn.Close()
	return &Info{
		User:           conn.AuthTag().Id(),
		ControllerName: controllerName,
		ControllerUUID: conn.ControllerTag().Id(),
		CACert:         cert,
		Endpoints:      getEndpoints(conn.APIHostPorts()),
	}, nil
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

// Info holds information about the Juju controller.
type Info struct {
	// User holds the name of the current local or external user.
	User string
	// ControllerName holds the name of the controller.
	ControllerName string
	// ControllerUUID holds the unique identifier for the Juju controller.
	ControllerUUID string
	// CACert is a security certificate for this controller.
	CACert string
	// Endpoints holds the addresses to use to connect to the Juju controller.
	Endpoints []string
}

// SetMacaroons sets the given macaroons as cookies in the given jar.
func SetMacaroons(jar http.CookieJar, macaroons map[string]macaroon.Slice) error {
	for uStr, ms := range macaroons {
		u, err := url.Parse(uStr)
		if err != nil {
			return errgo.Notef(err, "cannot parse macaroon URL %q", uStr)
		}
		cookie, err := httpbakery.NewCookie(candidNamespace, ms)
		if err != nil {
			return errgo.Notef(err, "cannot create cookie for %q", uStr)
		}
		jar.SetCookies(u, []*http.Cookie{cookie})
	}
	return nil
}

// TODO(frankban): this should really be provided by candid itself.
var candidNamespace = checkers.NewNamespace(map[string]string{"std": ""})

// MarshalAccounts encodes the given credentials information so that they are
// suitable for being used as the content of the Juju accounts.yaml file.
func MarshalAccounts(controller, user, password string) ([]byte, error) {
	accounts := struct {
		Controllers map[string]jujuclient.AccountDetails `yaml:"controllers"`
	}{
		Controllers: map[string]jujuclient.AccountDetails{
			controller: {
				User:     user,
				Password: password,
			},
		},
	}
	data, err := yaml.Marshal(accounts)
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal accounts")
	}
	return data, nil
}

// MarshalControllers encodes the given controller information so that it is
// suitable for being used as the content of the Juju controllers.yaml file.
func MarshalControllers(info *Info) ([]byte, error) {
	cs := jujuclient.Controllers{
		Controllers: map[string]jujuclient.ControllerDetails{
			info.ControllerName: {
				ControllerUUID: info.ControllerUUID,
				APIEndpoints:   info.Endpoints,
				CACert:         info.CACert,
			},
		},
		CurrentController: info.ControllerName,
	}
	data, err := yaml.Marshal(cs)
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal controllers")
	}
	return data, nil
}

// getEndpoints converts the given host and ports to simple string endpoints.
func getEndpoints(hostports [][]network.HostPort) (endpoints []string) {
	for _, hps := range hostports {
		for _, hp := range hps {
			endpoints = append(endpoints, hp.String())
		}
	}
	return endpoints
}

// controllerName holds the name assigned locally to the Juju controller.
const controllerName = "ctrl"

// apiOpen is defined as a variable for testing.
var apiOpen = func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}
