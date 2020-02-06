// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/juju/jujushell/internal/lxdclient"
	"github.com/juju/jujushell/internal/registry"
)

var newTests = []struct {
	about                  string
	client                 *client
	clientError            string
	expectedAfterFuncCalls int
	expectedCalls          [][]string
	expectedError          string
}{{
	about:         "error connecting to LXD",
	clientError:   "bad wolf",
	expectedError: "cannot connect to LXD: bad wolf",
}, {
	about: "error retrieving containers",
	client: &client{
		allError: errors.New("bad wolf"),
	},
	expectedCalls: [][]string{
		call("All"),
	},
	expectedError: "cannot retrieve initial containers: bad wolf",
}, {
	about:  "success",
	client: &client{},
	expectedCalls: [][]string{
		call("All"),
	},
}, {
	about: "success with existing container instances",
	client: &client{
		allResult: []*container{
			newContainer("c1", true, nil),
			newContainer("c2", false, nil),
			newContainer("c3", true, nil),
		},
	},
	expectedAfterFuncCalls: 2,
	expectedCalls: [][]string{
		call("All"),
		call("(c1).Started"),
		call("(c1).Name"),
		call("(c2).Started"),
		call("(c3).Started"),
		call("(c3).Name"),
	},
}}

func TestNew(t *testing.T) {
	c := qt.New(t)
	for _, test := range newTests {
		c.Run(test.about, func(c *qt.C) {
			// Patch the LXD client connection.
			c.Patch(registry.LXDutilsConnect, func(socket string) (lxdclient.Client, error) {
				c.Assert(socket, qt.Equals, socketPath)
				if test.clientError != "" {
					return nil, errors.New(test.clientError)
				}
				return test.client, nil
			})

			// Patch the time.AfterFunc call.
			var afterFuncCalls int
			c.Patch(registry.TimeAfterFunc, func(d time.Duration, f func()) *time.Timer {
				c.Assert(d, qt.Equals, duration)
				afterFuncCalls++
				return &time.Timer{}
			})

			// Run the test.
			r, err := registry.New(duration, socketPath)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(r, qt.IsNil)
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(r, qt.Not(qt.IsNil))
			}
			c.Assert(afterFuncCalls, qt.Equals, test.expectedAfterFuncCalls)
			if test.client != nil {
				c.Assert(test.client.calls, qt.DeepEquals, test.expectedCalls)
			}
		})
	}
}

func TestGet(t *testing.T) {
	c := qt.New(t)
	defer c.Done()

	// Patch lxdutils.Connect and time.AfterFunc calls.
	cl := client{
		getResult: newContainer("my-container", true, nil),
	}
	c.Patch(registry.LXDutilsConnect, func(socket string) (lxdclient.Client, error) {
		return &cl, nil
	})
	var timeoutFunc func()
	c.Patch(registry.TimeAfterFunc, func(d time.Duration, f func()) *time.Timer {
		timeoutFunc = f
		return &time.Timer{}
	})

	//  Create a registry.
	r, err := registry.New(duration, socketPath)
	c.Assert(err, qt.Equals, nil)

	// Get an active container.
	ac := r.Get("my-container")
	c.Assert(ac.Name(), qt.Equals, "my-container")

	// Ensure that running the timeout function stops the container.
	c.Assert(timeoutFunc, qt.Not(qt.IsNil))
	timeoutFunc()
	c.Assert(cl.calls, qt.DeepEquals, [][]string{
		call("All"),
		call("Get", "my-container"),
		call("(my-container).Started"),
		call("(my-container).Stop"),
	})

	// Try again with a container already stopped.
	cl.calls = nil
	timeoutFunc()
	c.Assert(cl.calls, qt.DeepEquals, [][]string{
		call("Get", "my-container"),
		call("(my-container).Started"),
	})
}

// client implements lxdclient.Client for testing.
type client struct {
	lxdclient.Client

	allResult []*container
	allError  error

	getResult *container
	getError  error

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
	if cl.getResult != nil {
		cl.getResult.client = cl
	}
	return cl.getResult, cl.getError
}

// newContainer creates and returns a new testing container.
func newContainer(name string, started bool, stopErr error) *container {
	return &container{
		name:    name,
		started: started,
		stopErr: stopErr,
	}
}

// container implements lxdclient.Container for testing.
type container struct {
	lxdclient.Container

	client *client

	name    string
	started bool
	stopErr error
}

func (c *container) register(name string, args ...string) {
	name = fmt.Sprintf("(%s).%s", c.name, name)
	c.client.register(name, args...)
}

func (c *container) Name() string {
	c.register("Name")
	return c.name
}

func (c *container) Started() bool {
	c.register("Started")
	return c.started
}

func (c *container) Stop() error {
	c.register("Stop")
	c.started = false
	return c.stopErr
}

func call(name string, args ...string) []string {
	return append([]string{name}, args...)
}

// duration is the timeout duration used in tests.
var duration = 42 * time.Second

// socketPath is the path to the LXD socket used in tests.
const socketPath = "/path/to/lxd.socket"
