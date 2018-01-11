// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"sort"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/juju/jujushell/internal/lxdutils"
)

// collect removes container instances if the number of containers is more than
// cap or the container has not been connected for the given number of days.
// If cap is 0, then all containers that are not connected are removed.
// Id days is 0, then old containers are only removed according to cap rules.
func collect(db storage, cap, days int) {
	group.Do("gc", func() (interface{}, error) {
		collect0(db, cap, days)
		return nil, nil
	})
}

func collect0(db storage, cap, days int) {
	log.Debugw("gc: running", "cap", cap, "days", days)
	defer log.Debug("gc: completed")

	// Connect to the LXD server.
	lxdclient, err := lxdutils.Connect()
	if err != nil {
		log.Errorw("gc: cannot connect to LXD server", "error", err.Error())
		return
	}

	// Retrieve the container instances present on the system.
	cs, err := lxdclient.All()
	if err != nil {
		log.Errorw("gc: cannot retrieve containers", "error", err.Error())
		return
	}
	if len(cs) <= cap && days == 0 {
		log.Debugw("gc: nothing to collect", "containers", len(cs))
		return
	}

	// Get information about current containers instances.
	containers := make([]*containerInfo, len(cs))
	for i, c := range cs {
		container := &containerInfo{
			name: c.Name(),
		}
		containers[i] = container
		addr, err := c.Addr()
		if err != nil {
			log.Errorw("gc: cannot retrieve container address", "error", err.Error(), "container", container.name)
			continue
		}
		container.addr = addr
		info, err := db.Info(addr)
		if err != nil {
			log.Errorw("gc: cannot retrieve container info", "error", err.Error(), "container", container.name)
			continue
		}
		container.numConnections = info.NumConnections
		container.lastConnection = info.LastConnection
	}

	// Sort the containers so that more likely to be collected come first.
	sort.Slice(containers, func(i, j int) bool {
		c1, c2 := containers[i], containers[j]
		if c1.numConnections != c2.numConnections {
			return c1.numConnections < c2.numConnections
		}
		return c1.lastConnection.Before(c2.lastConnection)
	})

	// Remove containers based on cap.
	toBeRemoved := make([]*containerInfo, 0, len(containers))
	for i := 0; i < len(containers)-cap; i++ {
		toBeRemoved = append(toBeRemoved, containers[0])
		containers = containers[1:]
	}

	// Remove containers base on days.
	t := time.Now().AddDate(0, 0, -days)
	for _, container := range containers {
		if container.numConnections == 0 && container.lastConnection.Before(t) {
			toBeRemoved = append(toBeRemoved, container)
		}
	}

	// Actually run the garbage collection.
	for _, c := range toBeRemoved {
		log.Debugw(
			"gb: removing container",
			"container", c.name,
			"address", c.addr,
			"num-connections", c.numConnections,
			"last-connection", c.lastConnection)
		if err = lxdutils.Cleanup(lxdclient, c.name); err != nil {
			log.Errorw("gc: cannot remove container", "error", err.Error(), "container", c.name)
			continue
		}
		log.Debugw("gb: removed container", "container", c.name)
		// If the container has an address, also remove any remaining
		// references in the db.
		if c.addr == "" {
			continue
		}
		for i := 0; i < c.numConnections; i++ {
			db.RemoveConn(c.addr)
		}
	}
}

type containerInfo struct {
	name           string
	addr           string
	numConnections int
	lastConnection time.Time
}

// group holds the namespace used for executing tasks suppressing duplicates.
var group = &singleflight.Group{}
