// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jujushell"
	"github.com/CanonicalLtd/jujushell/config"
	"github.com/CanonicalLtd/jujushell/internal/logging"
)

// main starts the Juju Shell server.
func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s <config path>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	if err := serve(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// serve serves the Juju shell.
func serve(configPath string) error {
	conf, err := config.Read(configPath)
	if err != nil {
		return errgo.Notef(err, "cannot read configuration file")
	}
	if err := logging.Setup(conf.LogLevel); err != nil {
		return errgo.Notef(err, "cannot set up logging")
	}
	log := logging.Logger()
	defer log.Sync()
	log.Infow("starting the server", "log level", conf.LogLevel, "port", conf.Port)
	server, err := jujushell.NewServer(jujushell.Params{
		JujuAddrs: conf.JujuAddrs,
	})
	if err != nil {
		return errgo.Notef(err, "cannot create new server")
	}
	// TODO: TLS configuration.
	if err := http.ListenAndServe(":8047", server); err != nil {
		return errgo.Notef(err, "cannot start the server")
	}
	return nil
}
