// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"go.uber.org/zap/zapcore"
	"gopkg.in/errgo.v1"
	"gopkg.in/yaml.v2"
)

// Config holds the server configuration.
type Config struct {
	// JujuAddrs holds the addresses of the current Juju controller.
	JujuAddrs []string `yaml:"juju-addrs"`
	// TLSCert and TLSKey hold TLS info for running the server.
	TLSCert string `yaml:"tls-cert"`
	TLSKey  string `yaml:"tls-key"`
	// LogLevel holds the logging level to use when running the server.
	LogLevel zapcore.Level `yaml:"log-level"`
	// Port holds the port on which the server will start listening.
	Port int
}

// Read reads the configuration options from a file at the given path.
func Read(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open %q", path)
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read %q", path)
	}
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse %q", path)
	}
	if err := validate(config); err != nil {
		return nil, errgo.Notef(err, "invalid configuration at %q", path)
	}
	return &config, nil
}

// validate validates the configuration options.
func validate(c Config) error {
	var missing []string
	if len(c.JujuAddrs) == 0 {
		missing = append(missing, "juju-addrs")
	}
	if c.Port <= 0 {
		missing = append(missing, "port")
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing fields %s", strings.Join(missing, ", "))
	}
	return nil
}
