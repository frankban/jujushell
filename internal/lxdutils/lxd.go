// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"golang.org/x/sync/singleflight"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jujushell/internal/juju"
)

const (
	// lxdSocket holds the path to the LXD socket provided by snapped LXD.
	lxdSocket = "/var/snap/lxd/common/lxd/unix.socket"
	// jujuCookie holds the path to the Juju cookie file in the container.
	jujuCookie = "/home/ubuntu/.local/share/juju/cookies/jimm.jujucharms.com.json"
)

// group holds the namespace used for executing tasks suppressing duplicates.
var group = &singleflight.Group{}

// Connect establishes a connection to the local snapped LXD server.
func Connect() (lxd.ContainerServer, error) {
	srv, err := lxd.ConnectLXDUnix(lxdSocket, nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot connect to local LXD server")
	}
	return srv, nil
}

// Ensure ensures that an LXD is available for the given username, and returns
// its address. If the container is not available, one is created using the
// given image, which is assumed to have juju already installed.
func Ensure(srv lxd.ContainerServer, image, username string, creds *juju.Credentials) (string, error) {
	containerName := containerName(username)

	_, err, _ := group.Do(containerName, func() (interface{}, error) {
		// Check for existing container.
		containers, err := srv.GetContainers()
		if err != nil {
			return nil, errgo.Notef(err, "cannot get containers")
		}
		var created, started bool
		for _, container := range containers {
			// If container exists, check if it's started.
			if containerName == container.Name {
				created = true
				started = container.Status != "Stopped"
			}
		}
		// Create and start the container if required.
		if !created {
			if err = createContainer(containerName, image, srv); err != nil {
				return nil, errgo.Mask(err)
			}
			// Prepare the Juju data directory in the container.
			if err = prepareContainer(containerName, srv, creds); err != nil {
				return nil, errgo.Mask(err)
			}
		}
		if !started {
			if err = startContainer(containerName, srv); err != nil {
				return nil, errgo.Mask(err)
			}
		}
		return nil, nil
	})
	if err != nil {
		// Ignore possible errors occurring while cleaning up failed container.
		srv.DeleteContainer(containerName)
		return "", errgo.Mask(err)
	}

	// Retrieve and return the container address.
	addr, err := containerAddr(containerName, srv)
	if err != nil {
		return "", errgo.Mask(err)
	}
	return addr, nil
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

// createContainer creates a container with the given name using the given
// image name.
func createContainer(containerName, image string, srv lxd.ContainerServer) error {
	// Get LXD to create the container.
	req := api.ContainersPost{
		Name: containerName,
		Source: api.ContainerSource{
			Type:  "image",
			Alias: image,
		},
	}
	op, err := srv.CreateContainer(req)
	if err != nil {
		return errgo.Notef(err, "cannot create container")
	}

	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "create container operation failed")
	}
	return nil
}

// startContainer starts the container with the given name.
func startContainer(containerName string, srv lxd.ContainerServer) error {
	// Get LXD to start the container.
	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}
	op, err := srv.UpdateContainerState(containerName, req, "")
	if err != nil {
		return errgo.Notef(err, "cannot start container")
	}

	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Notef(err, "start container operation failed")
	}
	return nil
}

// prepareContainer sets up dynamic container contents, like the Juju data
// directory which is user specific.
func prepareContainer(containerName string, srv lxd.ContainerServer, creds *juju.Credentials) error {
	if err := writeFile(containerName, srv, jujuCookie, []byte("helloworld")); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// containerAddr returns the ip address of the container with the given name.
// It assumes the container will be up and running in at most 30 seconds.
func containerAddr(containerName string, srv lxd.ContainerServer) (string, error) {
	for i := 0; i < 300; i++ {
		state, _, err := srv.GetContainerState(containerName)
		if err != nil {
			return "", errgo.Notef(err, "cannot get container state for %q", containerName)
		}
		network := state.Network["eth0"]
		for _, addr := range network.Addresses {
			if addr.Family == "inet" && addr.Scope == "global" && addr.Address != "" {
				return addr.Address, nil
			}
		}
		sleep(100 * time.Millisecond)
	}
	return "", errgo.Newf("cannot find address for %q", containerName)
}

// sleep is defined as a variable for testing purposes.
var sleep = func(d time.Duration) {
	time.Sleep(d)
}

// writeFile creates a file in the container with the given name, at the given
// path and with the given byte content. If the directory in which the file
// lives does not exist, it is recursively created.
func writeFile(containerName string, srv lxd.ContainerServer, path string, content []byte) error {
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
		if _, resp, err := srv.GetContainerFile(containerName, dir); err == nil {
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
		if err := srv.CreateContainerFile(containerName, dir, lxd.ContainerFileArgs{
			Type: "directory",
			UID:  uid,
			GID:  gid,
			Mode: 0700,
		}); err != nil {
			return errgo.Notef(err, "cannot create directory %q in the container", dir)
		}
	}
	if err := srv.CreateContainerFile(containerName, path, lxd.ContainerFileArgs{
		Content: bytes.NewReader(content),
		UID:     uid,
		GID:     gid,
		Mode:    0600,
	}); err != nil {
		return errgo.Notef(err, "cannot create file %q in the container", path)
	}
	return nil
}
