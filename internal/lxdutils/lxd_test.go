// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils_test

import (
	"errors"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"

	"github.com/CanonicalLtd/jujushell/internal/lxdutils"
)

var internalEnsureTests = []struct {
	about string
	srv   *srv
	user  string
	image string

	expectedAddr  string
	expectedError string

	expectedCreateReq    api.ContainersPost
	expectedUpdateReq    api.ContainerStatePut
	expectedUpdateName   string
	expectedGetStateName string
	expectedSleepCalls   int
}{{
	about: "error getting containers",
	srv: &srv{
		getError: errors.New("bad wolf"),
	},
	user:          "who",
	image:         "termserver",
	expectedError: "cannot get containers: bad wolf",
}, {
	about: "error creating the container",
	srv: &srv{
		createError: errors.New("bad wolf"),
	},
	user:          "who",
	image:         "termserver",
	expectedError: "cannot create container: bad wolf",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
}, {
	about: "error in the operation of creating the container",
	srv: &srv{
		createOpError: errors.New("bad wolf"),
	},
	user:          "rose",
	image:         "termserver",
	expectedError: "create container operation failed: bad wolf",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
}, {
	about: "error starting the container",
	srv: &srv{
		updateError: errors.New("bad wolf"),
	},
	user:          "who",
	image:         "termserver",
	expectedError: "cannot start container: bad wolf",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error in the operation of starting the container",
	srv: &srv{
		updateOpError: errors.New("bad wolf"),
	},
	user:          "rose",
	image:         "termserver",
	expectedError: "start container operation failed: bad wolf",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
}, {
	about: "error retrieving container state",
	srv: &srv{
		getStateError: errors.New("bad wolf"),
	},
	user:          "who",
	image:         "termserver",
	expectedError: `cannot get container state for "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about:         "error retrieving container address",
	srv:           &srv{},
	user:          "who",
	image:         "termserver",
	expectedError: `cannot find address for "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedSleepCalls:   300,
}, {
	about: "success",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet6",
			Scope:   "global",
		}, {
			Address: "1.2.3.5",
			Family:  "inet",
			Scope:   "local",
		}, {
			Address: "1.2.3.6",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	user:         "rose",
	image:        "termserver",
	expectedAddr: "1.2.3.6",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetStateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
}, {
	about: "success with other containers around",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-dalek",
			Status: "Stopped",
		}, {
			Name:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-cyberman",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet6",
			Scope:   "global",
		}, {
			Address: "1.2.3.5",
			Family:  "inet",
			Scope:   "local",
		}, {
			Address: "1.2.3.6",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	user:         "rose",
	image:        "termserver",
	expectedAddr: "1.2.3.6",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetStateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
}, {
	about: "success with container already created",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-dalek",
			Status: "Stopped",
		}, {
			Name:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
			Status: "Stopped",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	user:         "who",
	image:        "termserver",
	expectedAddr: "1.2.3.4",
	expectedUpdateReq: api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	},
	expectedUpdateName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "success with container already started",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	user:                 "who",
	image:                "termserver",
	expectedAddr:         "1.2.3.4",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "success with container already started (external user)",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	user:                 "cyberman@external",
	image:                "termserver",
	expectedAddr:         "1.2.3.4",
	expectedGetStateName: "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
}, {
	about: "success with container already started (user with special chars)",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-424e4758d42a93486f262645f62cd28d41f42499-these-are-the--v",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	user:                 "these.are@the++voy_ages",
	image:                "termserver",
	expectedAddr:         "1.2.3.4",
	expectedGetStateName: "ts-424e4758d42a93486f262645f62cd28d41f42499-these-are-the--v",
}}

func TestEnsure(t *testing.T) {
	for _, test := range internalEnsureTests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)
			s := &sleeper{
				c: c,
			}
			restore := patchSleep(s.sleep)
			defer restore()

			addr, err := lxdutils.Ensure(test.srv, test.user, test.image)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(addr, qt.Equals, "")
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(addr, qt.Equals, test.expectedAddr)
			}
			c.Assert(test.srv.createReq, qt.DeepEquals, test.expectedCreateReq)
			c.Assert(test.srv.updateReq, qt.DeepEquals, test.expectedUpdateReq)
			c.Assert(test.srv.updateName, qt.Equals, test.expectedUpdateName)
			c.Assert(test.srv.getStateName, qt.Equals, test.expectedGetStateName)
			c.Assert(s.callCount, qt.Equals, test.expectedSleepCalls)
		})
	}
}

// srv implements lxd.ContainerServer for testing purposes.
type srv struct {
	lxd.ContainerServer

	containers []api.Container
	getError   error

	createReq     api.ContainersPost
	createError   error
	createOpError error

	updateName    string
	updateReq     api.ContainerStatePut
	updateError   error
	updateOpError error

	getStateName      string
	getStateAddresses []api.ContainerStateNetworkAddress
	getStateError     error
}

func (s *srv) GetContainers() ([]api.Container, error) {
	return s.containers, s.getError
}

func (s *srv) CreateContainer(req api.ContainersPost) (*lxd.Operation, error) {
	s.createReq = req
	if s.createError != nil {
		return nil, s.createError
	}
	return newOp(s.createOpError), nil
}

func (s *srv) UpdateContainerState(name string, req api.ContainerStatePut, ETag string) (*lxd.Operation, error) {
	s.updateName = name
	s.updateReq = req
	if s.updateError != nil {
		return nil, s.updateError
	}
	return newOp(s.updateOpError), nil
}

func (s *srv) GetContainerState(name string) (*api.ContainerState, string, error) {
	s.getStateName = name
	if s.getStateError != nil {
		return nil, "", s.getStateError
	}
	return &api.ContainerState{
		Network: map[string]api.ContainerStateNetwork{
			"eth0": {
				Addresses: s.getStateAddresses,
			},
		},
	}, "", nil
}

// newOp creates and return a new LXD operation whose Wait method returns the
// provided error.
func newOp(err error) *lxd.Operation {
	op := lxd.Operation{
		Operation: api.Operation{
			StatusCode: api.Success,
		},
	}
	if err != nil {
		op.StatusCode = api.Failure
		op.Err = err.Error()
	}
	return &op
}

// sleeper is used to patch time.Sleep.
type sleeper struct {
	c         *qt.C
	callCount int
}

func (s *sleeper) sleep(d time.Duration) {
	s.callCount++
	s.c.Assert(d, qt.Equals, 100*time.Millisecond)
}

// patchSleep patches the api.sleep variable so that it is possible to avoid
// sleeping in tests.
func patchSleep(f func(d time.Duration)) (restore func()) {
	original := *lxdutils.Sleep
	*lxdutils.Sleep = f
	return func() {
		*lxdutils.Sleep = original
	}
}
