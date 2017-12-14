// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	lxd "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"

	"github.com/juju/jujushell/internal/lxdclient"
)

var newTests = []struct {
	about         string
	srv           lxd.ContainerServer
	err           error
	expectedError string
}{{
	about: "successful connection to server",
	srv:   &srv{},
}, {
	about:         "failure connecting to the server",
	err:           errors.New("bad wolf"),
	expectedError: `cannot connect to LXD server at "testing-socket": bad wolf`,
}}

func TestNew(t *testing.T) {
	c := qt.New(t)
	for _, test := range newTests {
		c.Run(test.about, func(c *qt.C) {
			patchLXDConnectUnix(c, test.srv, test.err)
			client, err := lxdclient.New("testing-socket")
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(client, qt.IsNil)
				return
			}
			c.Assert(err, qt.Equals, nil)
			c.Assert(client, qt.Not(qt.IsNil))
		})
	}
}

var clientTests = []struct {
	about string
	srv   *srv
	test  func(c *qt.C, client lxdclient.Client, srv *srv)
}{{
	about: "All: error getting containers",
	srv: &srv{
		getContainersError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, client lxdclient.Client, _ *srv) {
		cs, err := client.All()
		c.Assert(err, qt.ErrorMatches, "cannot get containers: bad wolf")
		c.Assert(cs, qt.IsNil)
	},
}, {
	about: "All: success",
	srv: &srv{
		getContainersResult: []lxdapi.Container{{
			Name:   "container-1",
			Status: "Stopped",
		}, {
			Name:   "container-2",
			Status: "Running",
		}},
	},
	test: func(c *qt.C, client lxdclient.Client, _ *srv) {
		cs, err := client.All()
		c.Assert(err, qt.Equals, nil)
		c.Assert(cs, qt.HasLen, 2)
		c.Assert(cs[0].Name(), qt.Equals, "container-1")
		c.Assert(cs[0].Started(), qt.Equals, false)
		c.Assert(cs[1].Name(), qt.Equals, "container-2")
		c.Assert(cs[1].Started(), qt.Equals, true)
	},
}, {
	about: "Get: failure getting container",
	srv:   &srv{},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		container, err := client.Get("no-such")
		c.Assert(err, qt.ErrorMatches, `cannot get container "no-such": not found`)
		c.Assert(container, qt.IsNil)
		c.Assert(srv.getContainerProvidedName, qt.Equals, "no-such")
	},
}, {
	about: "Get: success",
	srv: &srv{
		getContainersResult: []lxdapi.Container{{
			Name:   "container-1",
			Status: "Stopped",
		}, {
			Name:   "container-2",
			Status: "Running",
		}},
	},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		container, err := client.Get("container-2")
		c.Assert(err, qt.Equals, nil)
		c.Assert(container, qt.Not(qt.IsNil))
		c.Assert(container.Name(), qt.Equals, "container-2")
		c.Assert(srv.getContainerProvidedName, qt.Equals, "container-2")
	},
}, {
	about: "Create: failure",
	srv: &srv{
		createContainerError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		container, err := client.Create("my-image", "my-container", "default", "termserver-limited")
		c.Assert(err, qt.ErrorMatches, `cannot create container "my-container": bad wolf`)
		c.Assert(container, qt.IsNil)
		c.Assert(srv.createContainerProvidedReq, qt.DeepEquals, lxdapi.ContainersPost{
			ContainerPut: lxdapi.ContainerPut{
				Profiles: []string{"default", "termserver-limited"},
			},
			Name: "my-container",
			Source: lxdapi.ContainerSource{
				Type:  "image",
				Alias: "my-image",
			},
		})
	},
}, {
	about: "Create: operation failure",
	srv: &srv{
		createContainerOpError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		container, err := client.Create("my-image", "my-container", "default", "termserver-limited")
		c.Assert(err, qt.ErrorMatches, `cannot create container "my-container": operation failed: bad wolf`)
		c.Assert(container, qt.IsNil)
	},
}, {
	about: "Create: success",
	srv:   &srv{},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		container, err := client.Create("ubuntu:lts", "my-container", "default")
		c.Assert(err, qt.Equals, nil)
		c.Assert(container, qt.Not(qt.IsNil))
		c.Assert(container.Name(), qt.Equals, "my-container")
		c.Assert(container.Started(), qt.Equals, false)
		c.Assert(srv.createContainerProvidedReq, qt.DeepEquals, lxdapi.ContainersPost{
			ContainerPut: lxdapi.ContainerPut{
				Profiles: []string{"default"},
			},
			Name: "my-container",
			Source: lxdapi.ContainerSource{
				Type:  "image",
				Alias: "ubuntu:lts",
			},
		})
	},
}, {
	about: "Delete: failure",
	srv: &srv{
		deleteContainerError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		err := client.Delete("my-container")
		c.Assert(err, qt.ErrorMatches, `cannot delete container "my-container": bad wolf`)
		c.Assert(srv.deleteContainerProvidedName, qt.Equals, "my-container")
	},
}, {
	about: "Delete: operation failure",
	srv: &srv{
		deleteContainerOpError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		err := client.Delete("my-container")
		c.Assert(err, qt.ErrorMatches, `cannot delete container "my-container": operation failed: bad wolf`)
	},
}, {
	about: "Delete: success",
	srv:   &srv{},
	test: func(c *qt.C, client lxdclient.Client, srv *srv) {
		err := client.Delete("existing-container")
		c.Assert(err, qt.Equals, nil)
		c.Assert(srv.deleteContainerProvidedName, qt.Equals, "existing-container")
	},
}}

func TestClient(t *testing.T) {
	c := qt.New(t)
	for _, test := range clientTests {
		c.Run(test.about, func(c *qt.C) {
			patchLXDConnectUnix(c, test.srv, nil)
			client, err := lxdclient.New("testing-socket")
			c.Assert(err, qt.Equals, nil)
			test.test(c, client, test.srv)
		})
	}
}

var containerTests = []struct {
	about  string
	srv    *srv
	status string
	test   func(c *qt.C, container lxdclient.Container, srv *srv)
}{{
	about: "Name",
	srv:   &srv{},
	test: func(c *qt.C, container lxdclient.Container, _ *srv) {
		c.Assert(container.Name(), qt.Equals, "my-container")
	},
}, {
	about: "Addr: failure retrieving container state",
	srv: &srv{
		getContainerStateError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		s := &sleeper{
			c: c,
		}
		c.Patch(lxdclient.Sleep, s.sleep)
		addr, err := container.Addr()
		c.Assert(err, qt.ErrorMatches, `cannot get state for container "my-container": bad wolf`)
		c.Assert(addr, qt.Equals, "")
		c.Assert(s.callCount, qt.Equals, 0)
		c.Assert(srv.getContainerStateProvidedName, qt.Equals, "my-container")
	},
}, {
	about: "Addr: failure retrieving address",
	srv:   &srv{},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		s := &sleeper{
			c: c,
		}
		c.Patch(lxdclient.Sleep, s.sleep)
		addr, err := container.Addr()
		c.Assert(err, qt.ErrorMatches, `cannot find address for "my-container"`)
		c.Assert(addr, qt.Equals, "")
		c.Assert(s.callCount, qt.Equals, 300)
		c.Assert(srv.getContainerStateProvidedName, qt.Equals, "my-container")
	},
}, {
	about: "Addr: success",
	srv: &srv{
		getContainerStateAddresses: []lxdapi.ContainerStateNetworkAddress{{
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
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		s := &sleeper{
			c: c,
		}
		c.Patch(lxdclient.Sleep, s.sleep)
		addr, err := container.Addr()
		c.Assert(err, qt.Equals, nil)
		c.Assert(addr, qt.Equals, "1.2.3.6")
		c.Assert(s.callCount, qt.Equals, 0)
		c.Assert(srv.getContainerStateProvidedName, qt.Equals, "my-container")
	},
}, {
	about:  "Started: true",
	srv:    &srv{},
	status: "Running",
	test: func(c *qt.C, container lxdclient.Container, _ *srv) {
		c.Assert(container.Started(), qt.Equals, true)
	},
}, {
	about:  "Started: false",
	srv:    &srv{},
	status: "Stopped",
	test: func(c *qt.C, container lxdclient.Container, _ *srv) {
		c.Assert(container.Started(), qt.Equals, false)
	},
}, {
	about: "Start: failure",
	srv: &srv{
		updateContainerStateError: errors.New("bad wolf"),
	},
	status: "Stopped",
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.Start()
		c.Assert(err, qt.ErrorMatches, `cannot start container "my-container": bad wolf`)
		c.Assert(container.Started(), qt.Equals, false)
		c.Assert(srv.updateContainerStateProvidedName, qt.Equals, "my-container")
		c.Assert(srv.updateContainerStateProvidedReq, qt.DeepEquals, lxdapi.ContainerStatePut{
			Action:  "start",
			Timeout: -1,
		})
	},
}, {
	about: "Start: operation failure",
	srv: &srv{
		updateContainerStateOpError: errors.New("bad wolf"),
	},
	status: "Stopped",
	test: func(c *qt.C, container lxdclient.Container, _ *srv) {
		err := container.Start()
		c.Assert(err, qt.ErrorMatches, `cannot start container "my-container": operation failed: bad wolf`)
		c.Assert(container.Started(), qt.Equals, false)
	},
}, {
	about:  "Start: success",
	srv:    &srv{},
	status: "Stopped",
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.Start()
		c.Assert(err, qt.Equals, nil)
		c.Assert(container.Started(), qt.Equals, true)
		c.Assert(srv.updateContainerStateProvidedName, qt.Equals, "my-container")
		c.Assert(srv.updateContainerStateProvidedReq, qt.DeepEquals, lxdapi.ContainerStatePut{
			Action:  "start",
			Timeout: -1,
		})
	},
}, {
	about: "Stop: failure",
	srv: &srv{
		updateContainerStateError: errors.New("bad wolf"),
	},
	status: "Running",
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.Stop()
		c.Assert(err, qt.ErrorMatches, `cannot stop container "my-container": bad wolf`)
		c.Assert(container.Started(), qt.Equals, true)
		c.Assert(srv.updateContainerStateProvidedName, qt.Equals, "my-container")
		c.Assert(srv.updateContainerStateProvidedReq, qt.DeepEquals, lxdapi.ContainerStatePut{
			Action:  "stop",
			Timeout: -1,
		})
	},
}, {
	about: "Stop: operation failure",
	srv: &srv{
		updateContainerStateOpError: errors.New("bad wolf"),
	},
	status: "Running",
	test: func(c *qt.C, container lxdclient.Container, _ *srv) {
		err := container.Stop()
		c.Assert(err, qt.ErrorMatches, `cannot stop container "my-container": operation failed: bad wolf`)
		c.Assert(container.Started(), qt.Equals, true)
	},
}, {
	about:  "Stop: success",
	srv:    &srv{},
	status: "Running",
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.Stop()
		c.Assert(err, qt.Equals, nil)
		c.Assert(container.Started(), qt.Equals, false)
		c.Assert(srv.updateContainerStateProvidedName, qt.Equals, "my-container")
		c.Assert(srv.updateContainerStateProvidedReq, qt.DeepEquals, lxdapi.ContainerStatePut{
			Action:  "stop",
			Timeout: -1,
		})
	},
}, {
	about: "WriteFile: failure as a file in the path already exists",
	srv: &srv{
		getContainerFileResponses: []fileResponse{{}, {isFile: true}},
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.WriteFile("/example/path/to/file.yaml", []byte("data"))
		c.Assert(err, qt.ErrorMatches, `cannot create directory "/example/path": a file with the same name exists in the container`)
		c.Assert(srv.getContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.getContainerFileProvidedPaths, qt.DeepEquals, []string{"/example", "/example/path"})
		c.Assert(srv.createContainerFileProvidedName, qt.Equals, "")
		c.Assert(srv.createContainerFileProvidedPaths, qt.HasLen, 0)
		c.Assert(srv.createContainerFileProvidedArgs, qt.HasLen, 0)
	},
}, {
	about: "WriteFile: failure creating a directory",
	srv: &srv{
		createContainerFileErrors: []error{nil, errors.New("bad wolf")},
		getContainerFileResponses: []fileResponse{{}, {hasErr: true}, {hasErr: true}},
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.WriteFile("/example/path/to/file.yaml", []byte("data"))
		c.Assert(err, qt.ErrorMatches, `cannot create directory "/example/path/to" in the container: bad wolf`)
		c.Assert(srv.getContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.getContainerFileProvidedPaths, qt.DeepEquals, []string{"/example", "/example/path", "/example/path/to"})
		c.Assert(srv.createContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.createContainerFileProvidedPaths, qt.DeepEquals, []string{"/example/path", "/example/path/to"})
		c.Assert(srv.createContainerFileProvidedArgs, qt.CmpEquals(cmp.Comparer(createContainerFileArgsComparer)), []lxd.ContainerFileArgs{{
			UID:  42,
			GID:  47,
			Mode: 0700,
			Type: "directory",
		}, {
			UID:  42,
			GID:  47,
			Mode: 0700,
			Type: "directory",
		}})
	},
}, {
	about: "WriteFile: failure creating the file",
	srv: &srv{
		createContainerFileErrors: []error{errors.New("bad wolf")},
		getContainerFileResponses: []fileResponse{{}, {}, {}, {}},
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.WriteFile("/example/path/to/file.yaml", []byte("data"))
		c.Assert(err, qt.ErrorMatches, `cannot create file "/example/path/to/file.yaml" in the container: bad wolf`)
		c.Assert(srv.getContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.getContainerFileProvidedPaths, qt.DeepEquals, []string{"/example", "/example/path", "/example/path/to"})
		c.Assert(srv.createContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.createContainerFileProvidedPaths, qt.DeepEquals, []string{"/example/path/to/file.yaml"})
		c.Assert(srv.createContainerFileProvidedArgs, qt.CmpEquals(cmp.Comparer(createContainerFileArgsComparer)), []lxd.ContainerFileArgs{{
			Content: strings.NewReader("this is just a placeholder: see createContainerFileArgsComparer"),
			UID:     42,
			GID:     47,
			Mode:    0600,
		}})
	},
}, {
	about: "WriteFile: success",
	srv: &srv{
		createContainerFileErrors: []error{nil},
		getContainerFileResponses: []fileResponse{{}, {}, {}, {}},
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		err := container.WriteFile("/example/path/to/file.yaml", []byte("data"))
		c.Assert(err, qt.Equals, nil)
		c.Assert(srv.getContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.getContainerFileProvidedPaths, qt.DeepEquals, []string{"/example", "/example/path", "/example/path/to"})
		c.Assert(srv.createContainerFileProvidedName, qt.Equals, "my-container")
		c.Assert(srv.createContainerFileProvidedPaths, qt.DeepEquals, []string{"/example/path/to/file.yaml"})
		c.Assert(srv.createContainerFileProvidedArgs, qt.CmpEquals(cmp.Comparer(createContainerFileArgsComparer)), []lxd.ContainerFileArgs{{
			Content: strings.NewReader("this is just a placeholder: see createContainerFileArgsComparer"),
			UID:     42,
			GID:     47,
			Mode:    0600,
		}})
	},
}, {
	about: "Exec: failure",
	srv: &srv{
		execContainerError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		output, err := container.Exec("ls", "-l")
		c.Assert(err, qt.ErrorMatches, `cannot execute command "ls -l" on "my-container": bad wolf`)
		c.Assert(output, qt.Equals, "")
		c.Assert(srv.execContainerProvidedName, qt.Equals, "my-container")
		c.Assert(srv.execContainerProvidedReq, qt.DeepEquals, lxdapi.ContainerExecPost{
			Command:   []string{"ls", "-l"},
			WaitForWS: true,
		})
	},
}, {
	about: "Exec: operation failure",
	srv: &srv{
		execContainerOpError: errors.New("bad wolf"),
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		output, err := container.Exec("echo", "these are the voyages")
		c.Assert(err, qt.ErrorMatches, `cannot execute command "echo these are the voyages" on "my-container": operation failed: bad wolf`)
		c.Assert(output, qt.Equals, "")
	},
}, {
	about: "Exec: failure in the command exit code",
	srv: &srv{
		execContainerMetadata: map[string]interface{}{
			"return": float64(1),
		},
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		output, err := container.Exec("ls", "-l")
		c.Assert(err, qt.ErrorMatches, `command "ls -l" exited with code 1: test error`)
		c.Assert(output, qt.Equals, "")
	},
}, {
	about: "Exec: failure for invalid metadata",
	srv: &srv{
		execContainerMetadata: map[string]interface{}{
			"return": "bad wolf",
		},
	},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		output, err := container.Exec("ls", "-l")
		c.Assert(err, qt.ErrorMatches, "cannot retrieve retcode from exec operation metadata .*")
		c.Assert(output, qt.Equals, "")
	},
}, {
	about: "Exec: success",
	srv:   &srv{},
	test: func(c *qt.C, container lxdclient.Container, srv *srv) {
		output, err := container.Exec("echo", "these are the voyages")
		c.Assert(err, qt.Equals, nil)
		c.Assert(output, qt.Equals, "test output")
		c.Assert(srv.execContainerProvidedName, qt.Equals, "my-container")
		c.Assert(srv.execContainerProvidedReq, qt.DeepEquals, lxdapi.ContainerExecPost{
			Command:   []string{"echo", "these are the voyages"},
			WaitForWS: true,
		})
	},
}}

func TestContainer(t *testing.T) {
	c := qt.New(t)
	for _, test := range containerTests {
		c.Run(test.about, func(c *qt.C) {
			patchLXDConnectUnix(c, test.srv, nil)
			test.srv.getContainersResult = []lxdapi.Container{{
				Name:   "my-container",
				Status: test.status,
			}}
			client, err := lxdclient.New("testing-socket")
			c.Assert(err, qt.Equals, nil)
			container, err := client.Get("my-container")
			c.Assert(err, qt.Equals, nil)
			c.Assert(container, qt.Not(qt.IsNil))
			test.test(c, container, test.srv)
		})
	}
}

// srv implements lxd.ContainerServer for testing purposes.
type srv struct {
	lxd.ContainerServer

	getContainersResult      []lxdapi.Container
	getContainersError       error
	getContainerProvidedName string

	createContainerError       error
	createContainerOpError     error
	createContainerProvidedReq lxdapi.ContainersPost

	deleteContainerError        error
	deleteContainerOpError      error
	deleteContainerProvidedName string

	getContainerStateAddresses    []lxdapi.ContainerStateNetworkAddress
	getContainerStateError        error
	getContainerStateProvidedName string

	updateContainerStateError        error
	updateContainerStateOpError      error
	updateContainerStateProvidedName string
	updateContainerStateProvidedReq  lxdapi.ContainerStatePut

	getContainerFileResponses     []fileResponse
	getContainerFileProvidedName  string
	getContainerFileProvidedPaths []string

	createContainerFileErrors        []error
	createContainerFileProvidedName  string
	createContainerFileProvidedPaths []string
	createContainerFileProvidedArgs  []lxd.ContainerFileArgs

	execContainerError        error
	execContainerOpError      error
	execContainerMetadata     map[string]interface{}
	execContainerProvidedName string
	execContainerProvidedReq  lxdapi.ContainerExecPost
}

func (s *srv) GetContainers() ([]lxdapi.Container, error) {
	return s.getContainersResult, s.getContainersError
}

func (s *srv) GetContainer(name string) (container *lxdapi.Container, ETag string, err error) {
	s.getContainerProvidedName = name
	for _, container := range s.getContainersResult {
		if container.Name == name {
			return &container, "", nil
		}
	}
	return nil, "", errors.New("not found")
}

func (s *srv) CreateContainer(req lxdapi.ContainersPost) (*lxd.Operation, error) {
	s.createContainerProvidedReq = req
	if s.createContainerError != nil {
		return nil, s.createContainerError
	}
	return newOp(s.createContainerOpError, nil), nil
}

func (s *srv) DeleteContainer(name string) (*lxd.Operation, error) {
	s.deleteContainerProvidedName = name
	if s.deleteContainerError != nil {
		return nil, s.deleteContainerError
	}
	return newOp(s.deleteContainerOpError, nil), nil
}

func (s *srv) GetContainerState(name string) (*lxdapi.ContainerState, string, error) {
	s.getContainerStateProvidedName = name
	if s.getContainerStateError != nil {
		return nil, "", s.getContainerStateError
	}
	return &lxdapi.ContainerState{
		Network: map[string]lxdapi.ContainerStateNetwork{
			"eth0": {
				Addresses: s.getContainerStateAddresses,
			},
		},
	}, "", nil
}

func (s *srv) UpdateContainerState(name string, req lxdapi.ContainerStatePut, ETag string) (*lxd.Operation, error) {
	s.updateContainerStateProvidedName = name
	s.updateContainerStateProvidedReq = req
	if s.updateContainerStateError != nil {
		return nil, s.updateContainerStateError
	}
	return newOp(s.updateContainerStateOpError, nil), nil
}

func (s *srv) GetContainerFile(name, path string) (io.ReadCloser, *lxd.ContainerFileResponse, error) {
	if len(s.getContainerFileResponses) == 0 {
		panic("GetContainerFile: not enough responses available for " + path)
	}
	if s.getContainerFileProvidedName == "" {
		s.getContainerFileProvidedName = name
	}
	if s.getContainerFileProvidedName != name {
		panic("GetContainerFile: getting files from two different containers in the same request")
	}
	s.getContainerFileProvidedPaths = append(s.getContainerFileProvidedPaths, path)
	resp := s.getContainerFileResponses[0]
	s.getContainerFileResponses = s.getContainerFileResponses[1:]
	return resp.value()
}

func (s *srv) CreateContainerFile(name, path string, args lxd.ContainerFileArgs) error {
	if len(s.createContainerFileErrors) == 0 {
		panic("CreateContainerFile: not enough responses available for " + path)
	}
	if s.createContainerFileProvidedName == "" {
		s.createContainerFileProvidedName = name
	}
	if s.createContainerFileProvidedName != name {
		panic("CreateContainerFile: creating files from two different containers in the same request")
	}
	s.createContainerFileProvidedPaths = append(s.createContainerFileProvidedPaths, path)
	s.createContainerFileProvidedArgs = append(s.createContainerFileProvidedArgs, args)
	err := s.createContainerFileErrors[0]
	s.createContainerFileErrors = s.createContainerFileErrors[1:]
	return err
}

func (s *srv) ExecContainer(name string, req lxdapi.ContainerExecPost, args *lxd.ContainerExecArgs) (*lxd.Operation, error) {
	s.execContainerProvidedName = name
	s.execContainerProvidedReq = req
	args.Stdout.Write([]byte("test output"))
	args.Stderr.Write([]byte("test error"))
	if s.execContainerError != nil {
		return nil, s.execContainerError
	}
	if s.execContainerMetadata == nil {
		s.execContainerMetadata = map[string]interface{}{
			"return": float64(0),
		}
	}
	return newOp(s.execContainerOpError, s.execContainerMetadata), nil
}

// newOp creates and return a new LXD operation whose Wait method returns the
// provided error and metadata.
func newOp(err error, metadata map[string]interface{}) *lxd.Operation {
	op := lxd.Operation{
		Operation: lxdapi.Operation{
			Metadata:   metadata,
			StatusCode: lxdapi.Success,
		},
	}
	if err != nil {
		op.StatusCode = lxdapi.Failure
		op.Err = err.Error()
	}
	return &op
}

// fileResponse is used to build responses to
// lxd.ContainerServer.CreateContainerFile calls.
type fileResponse struct {
	isFile bool
	hasErr bool
}

func (r fileResponse) value() (io.ReadCloser, *lxd.ContainerFileResponse, error) {
	if r.hasErr {
		return nil, nil, errors.New("no such file")
	}
	resp := &lxd.ContainerFileResponse{
		UID:  42,
		GID:  47,
		Type: "directory",
	}
	if r.isFile {
		resp.Type = "file"
	}
	return nil, resp, nil
}

// createContainerFileArgsComparer is used to compare create file arguments.
func createContainerFileArgsComparer(a, b io.ReadSeeker) bool {
	return (a != nil && b != nil) || a == b
}

func patchLXDConnectUnix(c *qt.C, srv lxd.ContainerServer, err error) {
	c.Patch(lxdclient.LXDConnectUnix, func(path string, args *lxd.ConnectionArgs) (lxd.ContainerServer, error) {
		c.Assert(path, qt.Equals, "testing-socket")
		c.Assert(args, qt.IsNil)
		return srv, err
	})
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
