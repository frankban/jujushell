// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"strings"
	"time"

	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"golang.org/x/sync/singleflight"
	"gopkg.in/errgo.v1"
)

// lxdSocket holds the path to the LXD socket provided by snapped LXD.
const lxdSocket = "/var/snap/lxd/common/lxd/unix.socket"

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
func Ensure(srv lxd.ContainerServer, user, image string) (string, error) {
	// The "@" character cannot be included in LXD container names.
	containerName := "termserver-" + strings.Replace(user, "@", "-", 1)

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
		}
		if !started {
			if err = startContainer(containerName, srv); err != nil {
				return nil, errgo.Mask(err)
			}
		}
		return nil, nil
	})
	if err != nil {
		return "", errgo.Mask(err)
	}

	// Retrieve and return the container address.
	addr, err := containerAddr(containerName, srv)
	if err != nil {
		return "", errgo.Mask(err)
	}
	return addr, nil
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
