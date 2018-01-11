// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"

	cookiejar "github.com/juju/persistent-cookiejar"
	"golang.org/x/sync/singleflight"
	"gopkg.in/errgo.v1"

	"github.com/juju/jujushell/internal/juju"
	"github.com/juju/jujushell/internal/logging"
	"github.com/juju/jujushell/internal/lxdclient"
)

const (
	// lxdSocket holds the path to the LXD socket provided by snapped LXD.
	lxdSocket = "/var/snap/lxd/common/lxd/unix.socket"
	// jujuDataDir holds the directory used by Juju for its data.
	jujuDataDir = "/home/ubuntu/.local/share/juju"
)

var log = logging.Log()

// Connect establishes a connection to the local snapped LXD server.
func Connect() (lxdclient.Client, error) {
	client, err := lxdclient.New(lxdSocket)
	if err != nil {
		return nil, errgo.Notef(err, "cannot connect to local LXD server")
	}
	return client, nil
}

// Ensure ensures that an LXD is available for the given user, and returns its
// address. If the container is not available, one is created using the given
// image, which is assumed to have Juju already installed.
func Ensure(client lxdclient.Client, image string, profiles []string, info *juju.Info, creds *juju.Credentials) (addr string, err error) {
	name := containerName(info.User)

	defer func() {
		if err == nil {
			return
		}
		// If anything went wrong, just try to clean things up.
		log.Debugw("cleaning up due to error", "original error", err.Error())
		if cleanupErr := Cleanup(client, name); cleanupErr != nil {
			log.Debugw("cannot clean up container", "container", name, "error", cleanupErr.Error())
			return
		}
	}()

	container, err, _ := group.Do(name, func() (interface{}, error) {
		// Check for existing container.
		log.Debugw("getting containers")
		cs, err := client.All()
		if err != nil {
			return nil, errgo.Mask(err)
		}
		var c lxdclient.Container
		for _, container := range cs {
			// If container exists, check if it's started.
			if container.Name() == name {
				c = container
			}
		}
		// Create and start the container if required.
		if c == nil {
			log.Debugw("creating container", "container", name, "image", image)
			c, err = client.Create(image, name, profiles...)
			if err != nil {
				return nil, errgo.Mask(err)
			}
		}
		if !c.Started() {
			log.Debugw("starting container", "container", name)
			if err = c.Start(); err != nil {
				return nil, errgo.Mask(err)
			}
		}
		return c, nil
	})
	if err != nil {
		return "", errgo.Mask(err)
	}
	c := container.(lxdclient.Container)

	// Retrieve the container address.
	log.Debugw("retreiving container address", "container", name)
	addr, err = c.Addr()
	if err != nil {
		return "", errgo.Mask(err)
	}

	// Prepare the container, including the Juju data directory. This is done
	// every time, even if the container was already existing, in order, for
	// instance, to update credentials.
	log.Debugw("preparing container", "container", name, "address", addr)
	if err = prepare(c, info, creds); err != nil {
		return "", errgo.Mask(err)
	}
	return addr, nil
}

func Cleanup(client lxdclient.Client, name string) error {
	log.Debugw("cleaning up: retreiving container", "container", name)
	c, err := client.Get(name)
	if err != nil {
		return errgo.Notef(err, "cannot retreive container %q", name)
	}
	if c.Started() {
		// Ignore any errors from this point on, as there is nothing we can do.
		log.Debugw("cleaning up: tearing down the shell session", "container", name)
		if _, err = c.Exec("su", "-", "ubuntu", "-c", "~/.session teardown"); err != nil {
			log.Debugw("cleaning up: cannot tear down the shell session", "container", name, "error", err.Error())
		}
		log.Debugw("cleaning up: stopping container", "container", name)
		if err = c.Stop(); err != nil {
			log.Debugw("cleaning up: cannot stop the container", "container", name, "error", err.Error())
		}
	}
	log.Debugw("cleaning up: deleting container", "container", name)
	if err = client.Delete(name); err != nil {
		return errgo.Notef(err, "cannot delete container %q", name)
	}
	return nil
}

// prepare sets up dynamic container contents, like the Juju data directory
// which is user specific.
func prepare(c lxdclient.Container, info *juju.Info, creds *juju.Credentials) error {
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
		log.Debugw("writing macaroons to cookie jar", "container", c.Name())
		data, _ := jar.MarshalJSON() // MarshalJSON never fails.
		path := filepath.Join(jujuDataDir, "cookies", info.ControllerName+".json")
		if err = c.WriteFile(path, data); err != nil {
			return errgo.Notef(err, "cannot create cookie file in container %q", c.Name())
		}
	} else {
		// Prepare and save the accounts.yaml file in the container.
		data, err := juju.MarshalAccounts(info.ControllerName, creds.Username, creds.Password)
		if err != nil {
			return errgo.Notef(err, "cannot marshal Juju accounts")
		}
		log.Debugw("writing accounts.yaml", "container", c.Name())
		path := filepath.Join(jujuDataDir, "accounts.yaml")
		if err = c.WriteFile(path, data); err != nil {
			return errgo.Notef(err, "cannot create accounts file in container %q", c.Name())
		}
	}

	// Prepare and save the controllers.yaml file in the container.
	data, err := juju.MarshalControllers(info)
	if err != nil {
		return errgo.Notef(err, "cannot marshal Juju credentials")
	}
	log.Debugw("writing controllers.yaml", "container", c.Name())
	path := filepath.Join(jujuDataDir, "controllers.yaml")
	if err = c.WriteFile(path, data); err != nil {
		return errgo.Notef(err, "cannot create controllers file in container %q", c.Name())
	}

	// Run "juju login" in the container.
	log.Debugw("logging into Juju", "container", c.Name())
	output, err := c.Exec("su", "-", "ubuntu", "-c", "juju login -c "+info.ControllerName)
	if err != nil {
		return errgo.Notef(err, "cannot log into Juju in container %q", c.Name())
	}
	log.Debugw("successfully logged into Juju", "container", c.Name(), "output", output)

	// Initialize the shell session, including SSH keys.
	log.Debugw("initializing the shell session", "container", c.Name())
	output, err = c.Exec("su", "-", "ubuntu", "-c", "~/.session setup >> .session.log 2>&1")
	if err != nil {
		return errgo.Notef(err, "cannot initialize the shell session in container %q", c.Name())
	}
	log.Debugw("shell session successfully initialized", "container", c.Name(), "output", output)

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

// group holds the namespace used for executing tasks suppressing duplicates.
var group = &singleflight.Group{}
