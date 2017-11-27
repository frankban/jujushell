// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	cookiejar "github.com/juju/persistent-cookiejar"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"golang.org/x/sync/singleflight"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jujushell/internal/juju"
	"github.com/CanonicalLtd/jujushell/internal/logging"
)

const (
	// lxdSocket holds the path to the LXD socket provided by snapped LXD.
	lxdSocket = "/var/snap/lxd/common/lxd/unix.socket"
	// jujuDataDir holds the directory used by Juju for its data.
	jujuDataDir = "/home/ubuntu/.local/share/juju"
)

var log = logging.Log()

// Connect establishes a connection to the local snapped LXD server.
func Connect() (lxd.ContainerServer, error) {
	srv, err := lxd.ConnectLXDUnix(lxdSocket, nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot connect to local LXD server")
	}
	return srv, nil
}

// Ensure ensures that an LXD is available for the given user, and returns its
// address. If the container is not available, one is created using the given
// image, which is assumed to have Juju already installed.
func Ensure(srv lxd.ContainerServer, image string, info *juju.Info, creds *juju.Credentials) (addr string, err error) {
	container := newContainer(srv, info.User)
	defer func() {
		// If anything went wrong, just try to clean things up.
		if err != nil {
			container.delete()
		}
	}()

	err = singleFlight(container.name, func() error {
		// Check for existing container.
		cs, err := srv.GetContainers()
		if err != nil {
			return errgo.Notef(err, "cannot get containers")
		}
		var created, started bool
		for _, c := range cs {
			// If container exists, check if it's started.
			if c.Name == container.name {
				created = true
				started = c.Status != "Stopped"
			}
		}
		// Create and start the container if required.
		if !created {
			if err = container.create(image); err != nil {
				return errgo.Mask(err)
			}
		}
		if !started {
			if err = container.start(); err != nil {
				return errgo.Mask(err)
			}
		}
		return nil
	})
	if err != nil {
		return "", errgo.Mask(err)
	}

	// Retrieve the container address.
	addr, err = container.addr()
	if err != nil {
		return "", errgo.Mask(err)
	}

	// Prepare the Juju data directory in the container. This is done every
	// time, even if the container already exists, in order to update creds.
	log.Debugw("preparing juju", "container", container.name)
	if err = container.prepare(info, creds); err != nil {
		return "", errgo.Mask(err)
	}
	return addr, nil
}

// newContainer creates and returns a new container for the given user.
func newContainer(srv lxd.ContainerServer, username string) *container {
	return &container{
		name: containerName(username),
		srv:  srv,
	}
}

// container represents an LXD container instance.
type container struct {
	name string
	srv  lxd.ContainerServer
}

// create creates the container using the stored LXD image with the given name.
func (c *container) create(image string) error {
	log.Debugw("creating container", "container", c.name, "image", image)
	// Get LXD to create the container.
	req := api.ContainersPost{
		Name: c.name,
		Source: api.ContainerSource{
			Type:  "image",
			Alias: image,
		},
		ContainerPut: api.ContainerPut{
			Profiles: []string{"default", "termserver-limited"},
		},
	}
	op, err := c.srv.CreateContainer(req)
	if err != nil {
		return errgo.Notef(err, "cannot create container %q", c.name)
	}
	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "create container operation failed")
	}
	return nil
}

// start starts the container.
func (c *container) start() error {
	log.Debugw("starting container", "container", c.name)
	return errgo.Mask(c.updateState("start"))
}

// delete cleans up, stops and removes the container.
func (c *container) delete() error {
	log.Debugw("tearing down the shell session", "container", c.name)
	if err := c.exec("su", "-", "ubuntu", "-c", "~/.session teardown"); err != nil {
		// Ignore any execution errors.
		log.Debugw("cannot tear down the shell session", "container", c.name, "error", err.Error())
	}
	log.Debugw("stopping container", "container", c.name)
	if err := c.updateState("stop"); err != nil {
		// Ignore stop errors as we'll try to delete the container anyway.
		log.Debugw("cannot stop container", "container", c.name, "error", err.Error())
	}
	log.Debugw("deleting container", "container", c.name)
	if _, err := c.srv.DeleteContainer(c.name); err != nil {
		return errgo.Notef(err, "cannot delete container %q", c.name)
	}
	return nil
}

// prepare sets up dynamic container contents, like the Juju data directory
// which is user specific.
func (c *container) prepare(info *juju.Info, creds *juju.Credentials) error {
	if len(creds.Macaroons) != 0 {
		// Save authentication cookies in the container.
		jar, err := cookiejar.New(&cookiejar.Options{
			NoPersist: true,
		})
		if err != nil {
			return errgo.Notef(err, "cannot create cookie jar")
		}
		if err = juju.SetMacaroons(jar, creds.Macaroons); err != nil {
			return errgo.Notef(err, "cannot set macaroons in jar")
		}
		log.Debugw("writing macaroons to cookie jar", "container", c.name)
		data, _ := jar.MarshalJSON() // MarshalJSON never fails.
		path := filepath.Join(jujuDataDir, "cookies", info.ControllerName+".json")
		if err = c.writeFile(path, data); err != nil {
			return errgo.Notef(err, "cannot create cookie file in container %q", c.name)
		}
	} else {
		// Prepare and save the accounts.yaml file in the container.
		data, err := juju.MarshalAccounts(info.ControllerName, creds.Username, creds.Password)
		if err != nil {
			return errgo.Notef(err, "cannot marshal Juju accounts")
		}
		log.Debugw("writing accounts.yaml", "container", c.name)
		path := filepath.Join(jujuDataDir, "accounts.yaml")
		if err = c.writeFile(path, data); err != nil {
			return errgo.Notef(err, "cannot create accounts file in container %q", c.name)
		}
	}

	// Prepare and save the controllers.yaml file in the container.
	data, err := juju.MarshalControllers(info)
	if err != nil {
		return errgo.Notef(err, "cannot marshal Juju credentials")
	}
	log.Debugw("writing controllers.yaml", "container", c.name)
	path := filepath.Join(jujuDataDir, "controllers.yaml")
	if err = c.writeFile(path, data); err != nil {
		return errgo.Notef(err, "cannot create controllers file in container %q", c.name)
	}

	// Run "juju login" in the container.
	log.Debugw("logging into Juju", "container", c.name)
	if err = c.exec("su", "-", "ubuntu", "-c", "juju login -c "+info.ControllerName); err != nil {
		return errgo.Notef(err, "cannot log into Juju in container %q", c.name)
	}

	// Initialize the shell session, including SSH keys.
	log.Debugw("initializing the shell session", "container", c.name)
	if err = c.exec("su", "-", "ubuntu", "-c", "~/.session setup >> .session.log 2>&1"); err != nil {
		return errgo.Notef(err, "cannot initialize the shell session in container %q", c.name)
	}

	return nil
}

// writeFile creates a file in the container at the given path and with the
// given data. If the directory in which the file lives does not exist, it is
// recursively created.
func (c *container) writeFile(path string, data []byte) error {
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

// exec executes the given command in the container.
func (c *container) exec(cmd ...string) error {
	cmdstr := strings.Join(cmd, " ")
	// Do not execute the same command on the same container multiple times in
	// parallel.
	err := singleFlight(c.name+":"+cmdstr, func() error {
		req := api.ContainerExecPost{
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
			return errgo.Notef(err, "cannot execute command %q", cmdstr)
		}
		if err = op.Wait(); err != nil {
			return errgo.Notef(err, "execute command operation failed")
		}
		code, err := retcode(op)
		if err != nil {
			return errgo.Mask(err)
		}
		if code != 0 {
			return errgo.Newf("command %q exited with code %d: %s", cmdstr, code, stderr.String())
		}
		log.Debugw("succesfully executed command", "command", cmdstr, "stdout", stdout.String(), "stderr", stderr.String())
		return nil
	})
	return errgo.Mask(err)
}

// addr returns the ip address of the container. It assumes the container will
// be up and running in at most 30 seconds.
func (c *container) addr() (string, error) {
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

// updateState updates the state of the container.
func (c *container) updateState(action string) error {
	// Get LXD to start the container.
	req := api.ContainerStatePut{
		Action:  action,
		Timeout: -1,
	}
	op, err := c.srv.UpdateContainerState(c.name, req, "")
	if err != nil {
		return errgo.Notef(err, "cannot %s container %q", action, c.name)
	}
	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "%s container operation failed", action)
	}
	return nil
}

// containerName generates a container name for the given user name.
// The container name is unique for every user, so that stealing access is
// never possible.
func containerName(username string) string {
	sum := sha1.Sum([]byte(username))
	// Some characters cannot be included in LXD container names.
	r := strings.NewReplacer(
		"@", "-",
		"+", "-",
		".", "-",
		"_", "-",
	)
	name := fmt.Sprintf("ts-%x-%s", sum, r.Replace(username))
	// LXD containers have a limit of 63 characters for container names, which
	// seems a bit arbitrary. Anyway, cropping it at 60 should be safe enough.
	if len(name) > 60 {
		name = name[:60]
	}
	return name
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
func retcode(op *lxd.Operation) (int, error) {
	// See <https://github.com/lxc/lxd/blob/master/doc/rest-api.md#10containersnameexec>.
	switch v := op.Metadata["return"].(type) {
	// The concrete type for the retcode is float64, but it should really be an
	// int, so we are being defensive here.
	case int:
		return v, nil
	case float64:
		return int(v), nil
	}
	return 0, errgo.Newf("cannot retrieve retcode from exec operation metadata %v", op.Metadata)
}

// singleFlight executes the given function, making sure that only one execution
// is in-flight for a given key at a time. If a duplicate comes in, the caller
// waits for the original to complete and receives the same error.
func singleFlight(key string, f func() error) error {
	_, err, _ := group.Do(key, func() (interface{}, error) {
		if err := f(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

// group holds the namespace used for executing tasks suppressing duplicates.
var group = &singleflight.Group{}

// sleep is defined as a variable for testing purposes.
var sleep = func(d time.Duration) {
	time.Sleep(d)
}
