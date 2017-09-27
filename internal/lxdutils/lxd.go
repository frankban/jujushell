// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"time"

	"gopkg.in/errgo.v1"

	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
)

func getContainerAddr(containerName string, srv lxd.ContainerServer) (string, error) {
	for i := 0; i < 30; i++ {
		state, _, err := srv.GetContainerState(containerName)
		if err != nil {
			return "", errgo.Mask(err)
		}
		network := state.Network["eth0"]
		for _, addr := range network.Addresses {
			if addr.Family == "inet" && addr.Scope == "global" && addr.Address != "" {
				return addr.Address, nil

			}
		}
		time.Sleep(1 * time.Second)
	}
	return "", errgo.Newf("No address found for %s", containerName)
}

func createContainer(containerName string, imageName string, srv lxd.ContainerServer) error {
	req := api.ContainersPost{
		Name: containerName,
		Source: api.ContainerSource{
			Type:  "image",
			Alias: imageName,
		},
	}

	// Get LXD to create the container.
	op, err := srv.CreateContainer(req)
	if err != nil {
		return errgo.Mask(err)
	}

	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func startContainer(containerName string, srv lxd.ContainerServer) error {
	// Get LXD to start the container.
	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}

	op, err := srv.UpdateContainerState(containerName, req, "")
	if err != nil {
		return errgo.Mask(err)
	}

	// Wait for the operation to complete.
	if err = op.Wait(); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Ensure ensures that an LXD is available for the given username, and returns
// its address.
func Ensure(username string, imageName string) (string, error) {
	// Connect to LXD over the Unix socket.
	srv, err := lxd.ConnectLXDUnix("", nil)
	if err != nil {
		return "", errgo.Mask(err)
	}

	containerName := "termserver-" + username

	containers, err := srv.GetContainers()
	if err != nil {
		return "", errgo.Mask(err)
	}

	var created, started bool

	// Check for existing container.
	for _, container := range containers {
		// If container exists, check if it's started.
		if containerName == container.Name {
			created = true
			started = container.Status != "Stopped"
		}
	}

	if !created {
		err = createContainer(containerName, imageName, srv)
		if err != nil {
			return "", errgo.Mask(err)
		}
	}

	if !started {
		err = startContainer(containerName, srv)
		if err != nil {
			return "", errgo.Mask(err)
		}

	}

	address, err := getContainerAddr(containerName, srv)
	if err != nil {
		return "", errgo.Mask(err)
	}
	return address, nil
}
