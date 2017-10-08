// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"time"

	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"gopkg.in/errgo.v1"
)

// Ensure ensures that an LXD is available for the given username, and returns
// its address.
func Ensure(username string, imageName string) (string, error) {
	// Connect to LXD over the Unix socket.
	srv, err := lxd.ConnectLXDUnix("/var/snap/lxd/common/lxd/unix.socket", nil)
	if err != nil {
		return "", errgo.Mask(err)
	}
	containerName := "termserver-" + username

	// Check for existing container.
	containers, err := srv.GetContainers()
	if err != nil {
		return "", errgo.Mask(err)
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
		if err = createContainer(containerName, imageName, srv); err != nil {
			return "", errgo.Mask(err)
		}
	}
	if !started {
		if err = startContainer(containerName, srv); err != nil {
			return "", errgo.Mask(err)
		}
	}

	// Retrieve and return the container address.
	address, err := containerAddr(containerName, srv)
	if err != nil {
		return "", errgo.Mask(err)
	}
	return address, nil
}

// createContainer creates a container with the given name using the given
// image name.
func createContainer(containerName string, imageName string, srv lxd.ContainerServer) error {
	// Get LXD to create the container.
	req := api.ContainersPost{
		Name: containerName,
		Source: api.ContainerSource{
			Type:  "image",
			Alias: imageName,
		},
	}
	op, err := srv.CreateContainer(req)
	if err != nil {
		return errgo.Notef(err, "cannot create container")
	}

	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Mask(err)
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
		time.Sleep(100 * time.Millisecond)
	}
	return "", errgo.Newf("no address found for %q", containerName)
}
