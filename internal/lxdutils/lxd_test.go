// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils_test

import (
	"errors"
	"fmt"
	"testing"

	qt "github.com/frankban/quicktest"
	macaroon "gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jujushell/internal/juju"
	"github.com/CanonicalLtd/jujushell/internal/lxdclient"
	"github.com/CanonicalLtd/jujushell/internal/lxdutils"
)

var ensureTests = []struct {
	about  string
	client *client
	info   *juju.Info
	creds  *juju.Credentials

	expectedAddr  string
	expectedError string

	expectedCalls [][]string
}{{
	about: "error getting containers",
	client: &client{
		allError: errors.New("bad wolf"),
	},
	expectedError: "bad wolf",
	expectedCalls: [][]string{
		call("All"),
		// Cleaning up.
		call("Get", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"),
	},
}, {
	about: "error creating the container",
	client: &client{
		createError: errors.New("bad wolf"),
	},
	expectedError: "bad wolf",
	expectedCalls: [][]string{
		call("All"),
		call("Create", "termserver", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who", "default", "termserver-limited"),
		// Cleaning up.
		call("Get", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"),
	},
}, {
	about: "error starting the container",
	client: &client{
		startError: errors.New("bad wolf"),
	},
	expectedError: "bad wolf",
	expectedCalls: [][]string{
		call("All"),
		call("Create", "termserver", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who", "default", "termserver-limited"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Started"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Start"),
		// Cleaning up.
		call("Get", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Started"),
		call("Delete", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"),
	},
}, {
	about: "error retrieving container address",
	client: &client{
		addrError: errors.New("bad wolf"),
	},
	expectedError: "bad wolf",
	expectedCalls: [][]string{
		call("All"),
		call("Create", "termserver", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who", "default", "termserver-limited"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Started"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Start"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Addr"),
		// Cleaning up.
		call("Get", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Started"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-b7adf77905f540249517ca164255899e9ad1e2ac-who).Stop"),
		call("Delete", "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"),
	},
}, {
	about:  "error setting macaroons in the jar",
	client: &client{},
	info: &juju.Info{
		User: "dalek",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			":::": nil,
		},
	},
	expectedError: `cannot set macaroons in jar: cannot parse macaroon URL ":::": parse :::: missing protocol scheme`,
	expectedCalls: [][]string{
		call("All"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Addr"),
		// Cleaning up.
		call("Get", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Stop"),
		call("Delete", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
	},
}, {
	about: "error writing the cookie file",
	client: &client{
		writeFileErrors: []error{errors.New("bad wolf")},
	},
	info: &juju.Info{
		User:           "dalek",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create cookie file in container "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek": bad wolf`,
	expectedCalls: [][]string{
		call("All"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Addr"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile", "/home/ubuntu/.local/share/juju/cookies/my-controller.json", "null"),
		// Cleaning up.
		call("Get", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Stop"),
		call("Delete", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
	},
}, {
	about: "error writing the accounts file",
	client: &client{
		writeFileErrors: []error{errors.New("bad wolf")},
	},
	info: &juju.Info{
		User:           "dalek",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Username: "dalek@skaro",
		Password: "exterminate",
	},
	expectedError: `cannot create accounts file in container "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek": bad wolf`,
	expectedCalls: [][]string{
		call("All"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Addr"),
		call(
			"(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile",
			"/home/ubuntu/.local/share/juju/accounts.yaml",
			"controllers:\n  my-controller:\n    user: dalek@skaro\n    password: exterminate\n"),
		// Cleaning up.
		call("Get", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Stop"),
		call("Delete", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
	},
}, {
	about: "error writing controllers.yaml",
	client: &client{
		writeFileErrors: []error{nil, errors.New("bad wolf")},
	},
	info: &juju.Info{
		User:           "dalek",
		ControllerName: "my-controller",
		ControllerUUID: "ctrl-uuid",
		CACert:         "certificate",
		Endpoints:      []string{"1.2.3.4"},
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create controllers file in container "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek": bad wolf`,
	expectedCalls: [][]string{
		call("All"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Addr"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile", "/home/ubuntu/.local/share/juju/cookies/my-controller.json", "null"),
		call(
			"(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile",
			"/home/ubuntu/.local/share/juju/controllers.yaml",
			"controllers:\n  my-controller:\n    uuid: ctrl-uuid\n    api-endpoints: [1.2.3.4]\n    ca-cert: certificate\n    cloud: \"\"\n    controller-machine-count: 0\n    active-controller-machine-count: 0\ncurrent-controller: my-controller\n"),
		// Cleaning up.
		call("Get", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Stop"),
		call("Delete", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
	},
}, {
	about: "error logging into juju",
	client: &client{
		execErrors: []error{errors.New("bad wolf")},
	},
	info: &juju.Info{
		User:           "dalek",
		ControllerName: "my-controller",
		ControllerUUID: "ctrl-uuid",
		CACert:         "certificate",
		Endpoints:      []string{"1.2.3.4"},
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot log into Juju in container "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek": bad wolf`,
	expectedCalls: [][]string{
		call("All"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Addr"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile", "/home/ubuntu/.local/share/juju/cookies/my-controller.json", "null"),
		call(
			"(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile",
			"/home/ubuntu/.local/share/juju/controllers.yaml",
			"controllers:\n  my-controller:\n    uuid: ctrl-uuid\n    api-endpoints: [1.2.3.4]\n    ca-cert: certificate\n    cloud: \"\"\n    controller-machine-count: 0\n    active-controller-machine-count: 0\ncurrent-controller: my-controller\n"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "juju login -c my-controller"),
		// Cleaning up.
		call("Get", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Stop"),
		call("Delete", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
	},
}, {
	about: "error initializing the shell",
	client: &client{
		execErrors: []error{nil, errors.New("bad wolf")},
	},
	info: &juju.Info{
		User:           "dalek",
		ControllerName: "my-controller",
		ControllerUUID: "ctrl-uuid",
		CACert:         "certificate",
		Endpoints:      []string{"1.2.3.4"},
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot initialize the shell session in container "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek": bad wolf`,
	expectedCalls: [][]string{
		call("All"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Addr"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile", "/home/ubuntu/.local/share/juju/cookies/my-controller.json", "null"),
		call(
			"(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).WriteFile",
			"/home/ubuntu/.local/share/juju/controllers.yaml",
			"controllers:\n  my-controller:\n    uuid: ctrl-uuid\n    api-endpoints: [1.2.3.4]\n    ca-cert: certificate\n    cloud: \"\"\n    controller-machine-count: 0\n    active-controller-machine-count: 0\ncurrent-controller: my-controller\n"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "juju login -c my-controller"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session setup >> .session.log 2>&1"),
		// Cleaning up.
		call("Get", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Started"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Exec", "su", "-", "ubuntu", "-c", "~/.session teardown"),
		call("(ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek).Stop"),
		call("Delete", "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek"),
	},
}, {
	about:  "success",
	client: &client{},
	info: &juju.Info{
		User:           "rose",
		ControllerName: "my-controller",
		ControllerUUID: "ctrl-uuid",
		CACert:         "certificate",
		Endpoints:      []string{"1.2.3.4"},
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr: "1.2.3.6",
	expectedCalls: [][]string{
		call("All"),
		call("(ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose).Started"),
		call("(ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose).Addr"),
		call("(ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose).WriteFile", "/home/ubuntu/.local/share/juju/cookies/my-controller.json", "null"),
		call(
			"(ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose).WriteFile",
			"/home/ubuntu/.local/share/juju/controllers.yaml",
			"controllers:\n  my-controller:\n    uuid: ctrl-uuid\n    api-endpoints: [1.2.3.4]\n    ca-cert: certificate\n    cloud: \"\"\n    controller-machine-count: 0\n    active-controller-machine-count: 0\ncurrent-controller: my-controller\n"),
		call("(ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose).Exec", "su", "-", "ubuntu", "-c", "juju login -c my-controller"),
		call("(ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose).Exec", "su", "-", "ubuntu", "-c", "~/.session setup >> .session.log 2>&1"),
	},
}, {
	about:  "success with container stopped and external user",
	client: &client{},
	info: &juju.Info{
		User:           "cyberman@external",
		ControllerName: "ctrl",
		ControllerUUID: "ctrl-uuid",
		CACert:         "certificate",
		Endpoints:      []string{"1.2.3.7"},
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr: "1.2.3.7",
	expectedCalls: [][]string{
		call("All"),
		call("(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).Started"),
		call("(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).Start"),
		call("(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).Addr"),
		call("(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).WriteFile", "/home/ubuntu/.local/share/juju/cookies/ctrl.json", "null"),
		call(
			"(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).WriteFile",
			"/home/ubuntu/.local/share/juju/controllers.yaml",
			"controllers:\n  ctrl:\n    uuid: ctrl-uuid\n    api-endpoints: [1.2.3.7]\n    ca-cert: certificate\n    cloud: \"\"\n    controller-machine-count: 0\n    active-controller-machine-count: 0\ncurrent-controller: ctrl\n"),
		call("(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).Exec", "su", "-", "ubuntu", "-c", "juju login -c ctrl"),
		call("(ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa).Exec", "su", "-", "ubuntu", "-c", "~/.session setup >> .session.log 2>&1"),
	},
}, {
	about:  "success without machine and user with invalid characters",
	client: &client{},
	info: &juju.Info{
		User:           "d_a+l@e.k",
		ControllerName: "ctrl",
		ControllerUUID: "ctrl-uuid",
		CACert:         "certificate",
		Endpoints:      []string{"1.2.3.7"},
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr: "1.2.3.4",
	expectedCalls: [][]string{
		call("All"),
		call("Create", "termserver", "ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k", "default", "termserver-limited"),
		call("(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).Started"),
		call("(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).Start"),
		call("(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).Addr"),
		call("(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).WriteFile", "/home/ubuntu/.local/share/juju/cookies/ctrl.json", "null"),
		call(
			"(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).WriteFile",
			"/home/ubuntu/.local/share/juju/controllers.yaml",
			"controllers:\n  ctrl:\n    uuid: ctrl-uuid\n    api-endpoints: [1.2.3.7]\n    ca-cert: certificate\n    cloud: \"\"\n    controller-machine-count: 0\n    active-controller-machine-count: 0\ncurrent-controller: ctrl\n"),
		call("(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).Exec", "su", "-", "ubuntu", "-c", "juju login -c ctrl"),
		call("(ts-3c91974643169203624b07aa9d35afb0564d6103-d-a-l-e-k).Exec", "su", "-", "ubuntu", "-c", "~/.session setup >> .session.log 2>&1"),
	},
}}

func TestEnsure(t *testing.T) {
	for _, test := range ensureTests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)
			if test.info == nil {
				test.info = &juju.Info{
					User: "who",
				}
			}
			test.client.allResult = []*container{{
				name:    "ts-2f8dfb546853a3f551884e57e458533dfa5ad928-dalek",
				addr:    "1.2.3.5",
				started: true,
			}, {
				name:    "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
				addr:    "1.2.3.6",
				started: true,
			}, {
				name: "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
				addr: "1.2.3.7",
			}}
			addr, err := lxdutils.Ensure(test.client, "termserver", test.info, test.creds)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(addr, qt.Equals, "")
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(addr, qt.Equals, test.expectedAddr)
			}
			c.Assert(test.client.calls, qt.DeepEquals, test.expectedCalls)
		})
	}
}

// client implements lxdclient.Client for testing purposes.
type client struct {
	lxdclient.Client

	allResult []*container
	allError  error

	createError error
	startError  error
	stopError   error
	addrError   error

	writeFileErrors []error

	execOutput string
	execErrors []error

	calls [][]string
}

func (cl *client) register(name string, args ...string) {
	cl.calls = append(cl.calls, call(name, args...))
}

func (cl *client) All() ([]lxdclient.Container, error) {
	cl.register("All")
	result := make([]lxdclient.Container, len(cl.allResult))
	for i, container := range cl.allResult {
		container.client = cl
		result[i] = container
	}
	return result, cl.allError
}

func (cl *client) Get(name string) (lxdclient.Container, error) {
	cl.register("Get", name)
	for _, container := range cl.allResult {
		if container.name == name {
			container.client = cl
			return container, nil
		}
	}
	return nil, errors.New("not found")
}

func (cl *client) Create(image, name string, profiles ...string) (lxdclient.Container, error) {
	args := append([]string{image, name}, profiles...)
	cl.register("Create", args...)
	if cl.createError != nil {
		return nil, cl.createError
	}
	c := &container{
		client:  cl,
		name:    name,
		addr:    "1.2.3.4",
		started: false,
	}
	cl.allResult = append(cl.allResult, c)
	return c, nil
}

func (cl *client) Delete(name string) error {
	cl.register("Delete", name)
	return nil
}

// container implements lxdclient.Container for testing purposes.
type container struct {
	lxdclient.Container

	client *client

	name    string
	addr    string
	started bool
}

func (c *container) register(name string, args ...string) {
	name = fmt.Sprintf("(%s).%s", c.name, name)
	c.client.register(name, args...)
}

func (c *container) Name() string {
	return c.name
}

func (c *container) Addr() (string, error) {
	c.register("Addr")
	if c.client.addrError != nil {
		return "", c.client.addrError
	}
	return c.addr, nil
}

func (c *container) Started() bool {
	c.register("Started")
	return c.started
}

func (c *container) Start() error {
	c.register("Start")
	if c.client.startError != nil {
		return c.client.startError
	}
	c.started = true
	return nil
}

func (c *container) Stop() error {
	c.register("Stop")
	if c.client.stopError != nil {
		return c.client.stopError
	}
	c.started = false
	return nil
}

func (c *container) WriteFile(path string, data []byte) (err error) {
	c.register("WriteFile", path, string(data))
	if len(c.client.writeFileErrors) > 0 {
		err = c.client.writeFileErrors[0]
		c.client.writeFileErrors = c.client.writeFileErrors[1:]
	}
	return err
}

func (c *container) Exec(command string, args ...string) (output string, err error) {
	cmd := append([]string{command}, args...)
	c.register("Exec", cmd...)
	if len(c.client.execErrors) > 0 {
		err = c.client.execErrors[0]
		c.client.execErrors = c.client.execErrors[1:]
	}
	return c.client.execOutput, err
}

func call(name string, args ...string) []string {
	return append([]string{name}, args...)
}

func mustNewMacaroon(root string) *macaroon.Macaroon {
	m, err := macaroon.New([]byte(root), "id", "loc")
	if err != nil {
		panic(err)
	}
	return m
}
