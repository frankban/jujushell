// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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
	handler, err := jujushell.NewServer(jujushell.Params{
		ImageName: conf.ImageName,
		JujuAddrs: conf.JujuAddrs,
	})
	if err != nil {
		return errgo.Notef(err, "cannot create new server")
	}
	tlsConf, err := tlsConfig(conf.TLSCert, conf.TLSKey)
	if err != nil {
		return errgo.Notef(err, "cannot retrieve TLS configuration")
	}
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(conf.Port),
		Handler: handler,
	}
	if tlsConf != nil {
		server.TLSConfig = tlsConf
		return server.ListenAndServeTLS("", "")
	}
	return server.ListenAndServe()
}

// tlsConfig returns a TLS configuration for the given keys.
func tlsConfig(cert, key string) (*tls.Config, error) {
	if cert == "" && key == "" {
		return nil, nil
	}
	c, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return nil, errgo.Notef(err, "cannot create TLS certificate")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{c},
	}, nil
}
