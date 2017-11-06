// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"os"
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

var (
	// group holds the namespace used for executing tasks suppressing duplicates.
	group = &singleflight.Group{}
	log   = logging.Log()
)

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

	_, err, _ = group.Do(container.name, func() (interface{}, error) {
		// Check for existing container.
		cs, err := srv.GetContainers()
		if err != nil {
			return nil, errgo.Notef(err, "cannot get containers")
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
			log.Debugw("creating container", "container", container.name)
			if err = container.create(image); err != nil {
				return nil, errgo.Mask(err)
			}
		}
		if !started {
			log.Debugw("starting container", "container", container.name)
			if err = container.start(); err != nil {
				return nil, errgo.Mask(err)
			}
		}
		return nil, nil
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
	// Get LXD to create the container.
	req := api.ContainersPost{
		Name: c.name,
		Source: api.ContainerSource{
			Type:  "image",
			Alias: image,
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
	return errgo.Mask(c.updateState("start"))
}

// delete stops and removes the container.
func (c *container) delete() error {
	c.updateState("stop")
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
	return nil
}

// writeFile creates a file in the container at the given path and with the
// given data. If the directory in which the file lives does not exist, it is
// recursively created.
func (c *container) writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	numSegments := strings.Count(dir, "/")
	segments := make([]string, numSegments)
	for i := numSegments - 1; i >= 0; i-- {
		segments[i] = dir
		dir = filepath.Dir(dir)
	}
	var uid, gid int64
	// Recursively create directories if required.
	for _, dir := range segments {
		if _, resp, err := c.srv.GetContainerFile(c.name, dir); err == nil {
			// The directory exists.
			if resp.Type != "directory" {
				return errgo.Newf("cannot create directory %q: a file with the same name exists in the container", dir)
			}
			// If we are traversing the "/home/ubuntu" segment, store the ubuntu
			// user UID and GID for later use.
			if dir == "/home/ubuntu" {
				uid, gid = resp.UID, resp.GID
			}
			continue
		}
		if err := c.srv.CreateContainerFile(c.name, dir, lxd.ContainerFileArgs{
			Type: "directory",
			UID:  uid,
			GID:  gid,
			Mode: 0700,
		}); err != nil {
			return errgo.Notef(err, "cannot create directory %q in the container", dir)
		}
	}
	if err := c.srv.CreateContainerFile(c.name, path, lxd.ContainerFileArgs{
		Content: bytes.NewReader(data),
		UID:     uid,
		GID:     gid,
		Mode:    0600,
	}); err != nil {
		return errgo.Notef(err, "cannot create file %q in the container", path)
	}
	return nil
}

// exec executes the given command in the container.
func (c *container) exec(cmd ...string) error {
	req := api.ContainerExecPost{
		Command:   cmd,
		WaitForWS: true,
	}
	// TODO frankban: check that the cmd succeded, maybe by looking at stderr?
	args := lxd.ContainerExecArgs{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	op, err := c.srv.ExecContainer(c.name, req, &args)
	if err != nil {
		return errgo.Notef(err, "cannot execute command %q", strings.Join(cmd, " "))
	}
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "execute command operation failed")
	}
	return nil
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

// sleep is defined as a variable for testing purposes.
var sleep = func(d time.Duration) {
	time.Sleep(d)
}
