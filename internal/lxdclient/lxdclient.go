// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"time"

	lxd "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"
	"golang.org/x/sync/singleflight"
	errgo "gopkg.in/errgo.v1"
)

// Client describes an LXD client, which is used to create, delete and retrieve
// LXD containers.
type Client interface {
	// All returns all existing LXD containers.
	All() ([]Container, error)
	// Get returns the LXD container with the given name.
	Get(name string) (Container, error)
	// Create creates a container using the LXD image with the given name.
	Create(image, name string, profiles ...string) (Container, error)
	// Delete removes the container with the given name. It assumes the
	// container exists and is not running.
	Delete(name string) error
}

// Container describes an LXD container instance.
type Container interface {
	// Name returns the container name.
	Name() string
	// Addr returns the public ip address of the container.
	Addr() (string, error)
	// Started reports whether the container is running.
	Started() bool
	// Start starts the container.
	Start() error
	// Stop stops the container.
	Stop() error
	// WriteFile creates a file in the container at the given path and data.
	WriteFile(path string, data []byte) error
	// Exec executes the given command in the container and returns its output.
	Exec(command string, args ...string) (string, error)
}

// New returns an LXD client connected to the socket at the given path.
func New(socket string) (Client, error) {
	srv, err := lxdConnectUnix(socket, nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot connect to LXD server at %q", socket)
	}
	return &client{
		srv: srv,
	}, nil
}

// lxdConnectUnix is defined as a variable for testing purposes.
var lxdConnectUnix = func(path string, args *lxd.ConnectionArgs) (lxd.ContainerServer, error) {
	return lxd.ConnectLXDUnix(path, args)
}

// client implements Client.
type client struct {
	srv lxd.ContainerServer
}

// All returns all existing LXD containers.
func (cl *client) All() ([]Container, error) {
	cs, err := cl.srv.GetContainers()
	if err != nil {
		return nil, errgo.Notef(err, "cannot get containers")
	}
	containers := make([]Container, len(cs))
	for i, c := range cs {
		containers[i] = &container{
			name:    c.Name,
			started: c.Status != "Stopped",
			srv:     cl.srv,
		}
	}
	return containers, nil
}

// Get returns the LXD container with the given name.
func (cl *client) Get(name string) (Container, error) {
	c, _, err := cl.srv.GetContainer(name)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get container %q", name)
	}
	return &container{
		name:    c.Name,
		started: c.Status != "Stopped",
		srv:     cl.srv,
	}, nil
}

// Create creates a container using the LXD image with the given name.
func (cl *client) Create(image, name string, profiles ...string) (Container, error) {
	req := lxdapi.ContainersPost{
		Name: name,
		Source: lxdapi.ContainerSource{
			Type:  "image",
			Alias: image,
		},
		ContainerPut: lxdapi.ContainerPut{
			Profiles: profiles,
		},
	}
	op, err := cl.srv.CreateContainer(req)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create container %q", name)
	}
	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return nil, errgo.Notef(err, "cannot create container %q: operation failed", name)
	}
	return &container{
		name: name,
		srv:  cl.srv,
	}, nil
}

// Delete removes the container with the given name. It assumes the container
// exists and is not running.
func (cl *client) Delete(name string) error {
	op, err := cl.srv.DeleteContainer(name)
	if err != nil {
		return errgo.Notef(err, "cannot delete container %q", name)
	}
	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "cannot delete container %q: operation failed", name)
	}
	return nil
}

// container implements Container, and represents an LXD instance.
type container struct {
	name    string
	started bool
	srv     lxd.ContainerServer
}

// Name returns the container name.
func (c *container) Name() string {
	return c.name
}

// Addr returns the ip address of the container. It assumes the container will
// be up and running in at most 30 seconds.
func (c *container) Addr() (string, error) {
	for i := 0; i < 300; i++ {
		state, _, err := c.srv.GetContainerState(c.name)
		if err != nil {
			return "", errgo.Notef(err, "cannot get state for container %q", c.name)
		}
		network := state.Network["eth0"]
		for _, addr := range network.Addresses {
			if addr.Family == "inet" && addr.Scope == "global" && addr.Address != "" {
				return addr.Address, nil
			}
		}
		sleep(100 * time.Millisecond)
	}
	return "", errgo.Newf("cannot find address for %q", c.name)
}

// Started reports whether the container is running.
func (c *container) Started() bool {
	return c.started
}

// Start starts the container.
func (c *container) Start() error {
	if err := c.updateState("start"); err != nil {
		return errgo.Mask(err)
	}
	c.started = true
	return nil
}

// Stop stops the container.
func (c *container) Stop() error {
	if err := c.updateState("stop"); err != nil {
		return errgo.Mask(err)
	}
	c.started = false
	return nil
}

// WriteFile creates a file in the container at the given path and data. If the
// directory in which the file lives does not exist, it is recursively created.
func (c *container) WriteFile(path string, data []byte) error {
	uid, gid, err := c.mkdir(filepath.Dir(path))
	if err != nil {
		return errgo.Mask(err)
	}
	if err = c.srv.CreateContainerFile(c.name, path, lxd.ContainerFileArgs{
		Content: bytes.NewReader(data),
		UID:     uid,
		GID:     gid,
		Mode:    0600,
	}); err != nil {
		return errgo.Notef(err, "cannot create file %q in the container", path)
	}
	return nil
}

// Exec executes the given command in the container and returns its output.
func (c *container) Exec(command string, args ...string) (string, error) {
	cmd := append([]string{command}, args...)
	cmdstr := strings.Join(cmd, " ")
	// Do not execute the same command on the same container multiple times in
	// parallel.
	stdout, err, _ := group.Do(c.name+":"+cmdstr, func() (interface{}, error) {
		req := lxdapi.ContainerExecPost{
			Command:   cmd,
			WaitForWS: true,
		}
		var stdin, stdout, stderr bytes.Buffer
		args := lxd.ContainerExecArgs{
			Stdin:  readWriteNopCloser{&stdin},
			Stdout: readWriteNopCloser{&stdout},
			Stderr: readWriteNopCloser{&stderr},
		}
		op, err := c.srv.ExecContainer(c.name, req, &args)
		if err != nil {
			return "", errgo.Notef(err, "cannot execute command %q on %q", cmdstr, c.name)
		}
		if err = op.Wait(); err != nil {
			return "", errgo.Notef(err, "cannot execute command %q on %q: operation failed", cmdstr, c.name)
		}
		code, err := retcode(op)
		if err != nil {
			return "", errgo.Mask(err)
		}
		if code != 0 {
			return "", errgo.Newf("command %q exited with code %d: %s", cmdstr, code, stderr.String())
		}
		return stdout.String(), nil
	})
	if err != nil {
		return "", errgo.Mask(err)
	}
	return stdout.(string), nil
}

// updateState updates the state of the container.
func (c *container) updateState(action string) error {
	req := lxdapi.ContainerStatePut{
		Action:  action,
		Timeout: -1,
	}
	op, err := c.srv.UpdateContainerState(c.name, req, "")
	if err != nil {
		return errgo.Notef(err, "cannot %s container %q", action, c.name)
	}
	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "cannot %s container %q: operation failed", action, c.name)
	}
	return nil
}

// mkdir creates (if it does not exist) a directory in the container at the
// given path, and returns its uid, and gid.
func (c *container) mkdir(path string) (uid, gid int64, err error) {
	// idInfo holds user and group id information.
	type idInfo struct {
		uid, gid int64
	}
	// Creating the directory structure is done as a single flight.
	result, err, _ := group.Do(c.name+":"+path, func() (interface{}, error) {
		numSegments := strings.Count(path, "/")
		segments := make([]string, numSegments)
		for i := numSegments - 1; i >= 0; i-- {
			segments[i] = path
			path = filepath.Dir(path)
		}
		var ids idInfo
		// Recursively create directories if required.
		for _, dir := range segments {
			if _, resp, err := c.srv.GetContainerFile(c.name, dir); err == nil {
				// The directory exists.
				if resp.Type != "directory" {
					return nil, errgo.Newf("cannot create directory %q: a file with the same name exists in the container", dir)
				}
				// Store the uid and gid of the parent directory for later use.
				ids.uid, ids.gid = resp.UID, resp.GID
				continue
			}
			if err := c.srv.CreateContainerFile(c.name, dir, lxd.ContainerFileArgs{
				Type: "directory",
				UID:  ids.uid,
				GID:  ids.gid,
				Mode: 0700,
			}); err != nil {
				return nil, errgo.Notef(err, "cannot create directory %q in the container", dir)
			}
		}
		return &ids, nil
	})
	if err != nil {
		return 0, 0, errgo.Mask(err)
	}
	ids := result.(*idInfo)
	return ids.uid, ids.gid, nil
}

// readWriteNopCloser is used to add a noop Close method to a io.ReadWriter.
type readWriteNopCloser struct {
	io.ReadWriter
}

// Close implement io.Closer by doing nothing.
func (readWriteNopCloser) Close() error {
	return nil
}

// retcode returns the exit code from the command executed with the given op.
func retcode(op lxd.Operation) (int, error) {
	// See <https://github.com/lxc/lxd/blob/master/doc/rest-api.md#10containersnameexec>.
	meta := op.Get().Metadata
	switch v := meta["return"].(type) {
	// The concrete type for the retcode is float64, but it should really be an
	// int, so we are being defensive here.
	case int:
		return v, nil
	case float64:
		return int(v), nil
	}
	return 0, errgo.Newf("cannot retrieve retcode from exec operation metadata %v", meta)
}

// group holds the namespace used for executing tasks suppressing duplicates.
var group = &singleflight.Group{}

// sleep is defined as a variable for testing purposes.
var sleep = func(d time.Duration) {
	time.Sleep(d)
}
