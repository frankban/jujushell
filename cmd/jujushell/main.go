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

	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/errgo.v1"

	"github.com/juju/jujushell"
	"github.com/juju/jujushell/config"
	"github.com/juju/jujushell/internal/logging"
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
	log := logging.Log()
	log.SetLevel(conf.LogLevel)
	defer log.Sync()
	log.Infow("starting the server", "log level", conf.LogLevel, "port", conf.Port)
	handler, err := jujushell.NewServer(jujushell.Params{
		ImageName: conf.ImageName,
		JujuAddrs: conf.JujuAddrs,
		JujuCert:  conf.JujuCert,
		Profiles:  conf.Profiles,
	})
	if err != nil {
		return errgo.Notef(err, "cannot create new server")
	}
	tlsConf, err := tlsConfig(conf.TLSCert, conf.TLSKey, conf.DNSName)
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

// tlsConfig returns a TLS configuration for the given keys and DNS name.
// When the DNS name is not empty, Let's Encrypt is used to manage certs.
func tlsConfig(cert, key, name string) (*tls.Config, error) {
	if cert == "" && key == "" {
		if name == "" {
			return nil, nil
		}
		manager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(name),
			Cache:      autocert.DirCache("/tmp/certs"),
		}
		return &tls.Config{
			GetCertificate: manager.GetCertificate,
		}, nil
	}
	c, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return nil, errgo.Notef(err, "cannot create TLS certificate")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{c},
	}, nil
}
