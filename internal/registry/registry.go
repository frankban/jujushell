// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"sync"
	"time"

	"gopkg.in/errgo.v1"

	"github.com/juju/jujushell/internal/logging"
	"github.com/juju/jujushell/internal/lxdclient"
	"github.com/juju/jujushell/internal/lxdutils"
)

var log = logging.Log()

// New creates and returns a new registry for active containers. Containers are
// stopped after the provided duration. The LXD client is connected using the
// given socket path.
func New(d time.Duration, socketPath string) (*Registry, error) {
	client, err := lxdutilsConnect(socketPath)
	if err != nil {
		return nil, errgo.Notef(err, "cannot connect to LXD")
	}
	cs, err := client.All()
	if err != nil {
		return nil, errgo.Notef(err, "cannot retrieve initial containers")
	}
	r := Registry{
		d:          d,
		socketPath: socketPath,
		containers: make(map[string]*ActiveContainer, len(cs)),
	}
	for _, c := range cs {
		if c.Started() {
			r.Get(c.Name())
		}
	}
	return &r, nil
}

// Registry stores and keeps track of the currently active cobtainers. Use the
// Get method on the registry to retrieve a stored container or add a new one.
type Registry struct {
	d          time.Duration
	socketPath string
	mu         sync.Mutex
	containers map[string]*ActiveContainer
}

// Get returns the active container with the given name. The container is also
// stored in the registry if not already known.
func (r *Registry) Get(name string) *ActiveContainer {
	log.Debugw("current active containers", "containers", r.containers)
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.containers[name]
	if c == nil {
		c = &ActiveContainer{
			name: name,
			d:    r.d,
		}
		if r.d != 0 {
			c.timer = timeAfterFunc(r.d, func() {
				log.Debugw("stopping container for inactivity", "container", name)
				if err := r.stop(name); err != nil {
					log.Debugw("cannot stop container for inactivity", "container", name, "error", err.Error())
				}
			})
		}
		r.containers[name] = c
	}
	return c
}

// stop stops the container with the given name. It is usally called by a timer
// after a certain amount of time without any activity on the container.
func (r *Registry) stop(name string) error {
	client, err := lxdutilsConnect(r.socketPath)
	if err != nil {
		return errgo.Mask(err)
	}
	c, err := client.Get(name)
	if err != nil {
		return errgo.Mask(err)
	}
	if !c.Started() {
		return errgo.Newf("container %s is not started", name)
	}
	if err = c.Stop(); err != nil {
		return errgo.Mask(err)
	}
	r.mu.Lock()
	delete(r.containers, name)
	r.mu.Unlock()
	return nil
}

// ActiveContainer represents a container currently running.
type ActiveContainer struct {
	name  string
	d     time.Duration
	timer *time.Timer
}

// Name returns the name of the container.
func (c *ActiveContainer) Name() string {
	return c.name
}

// SetActive registers activity on the container.
func (c *ActiveContainer) SetActive() {
	if c.timer == nil {
		return
	}
	if c.timer.Stop() {
		c.timer.Reset(c.d)
	}
}

// lxdutilsConnect is defined as a variable for testing.
var lxdutilsConnect = func(socketPath string) (lxdclient.Client, error) {
	return lxdutils.Connect(socketPath)
}

// timeAfterFunc is defined as a variable for testing.
var timeAfterFunc = func(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}
