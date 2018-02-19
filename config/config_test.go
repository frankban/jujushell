// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"io/ioutil"
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v2"

	"github.com/juju/jujushell/config"
)

var readTests = []struct {
	about          string
	content        []byte
	expectedConfig *config.Config
	expectedError  string
}{{
	about: "valid config",
	content: mustMarshalYAML(map[string]interface{}{
		"allowed-users":   []string{"who", "dalek"},
		"image-name":      "myimage",
		"juju-addrs":      []string{"1.2.3.4", "4.3.2.1"},
		"juju-cert":       "my Juju cert",
		"log-level":       "debug",
		"port":            8047,
		"profiles":        []string{"default", "termserver"},
		"session-timeout": 42,
		"welcome-message": "exterminate!",
	}),
	expectedConfig: &config.Config{
		AllowedUsers:   []string{"who", "dalek"},
		ImageName:      "myimage",
		JujuAddrs:      []string{"1.2.3.4", "4.3.2.1"},
		JujuCert:       "my Juju cert",
		LogLevel:       zapcore.DebugLevel,
		Port:           8047,
		Profiles:       []string{"default", "termserver"},
		SessionTimeout: 42,
		WelcomeMessage: "exterminate!",
	},
}, {
	about: "valid minimum config",
	content: mustMarshalYAML(map[string]interface{}{
		"image-name": "myimage",
		"juju-addrs": []string{"1.2.3.4", "4.3.2.1"},
		"port":       8047,
		"profiles":   []string{"default", "termserver"},
	}),
	expectedConfig: &config.Config{
		ImageName: "myimage",
		JujuAddrs: []string{"1.2.3.4", "4.3.2.1"},
		Port:      8047,
		Profiles:  []string{"default", "termserver"},
	},
}, {
	about: "valid jaas config",
	content: mustMarshalYAML(map[string]interface{}{
		"image-name": "myimage",
		"juju-addrs": []string{"jimm.jujucharms.com:443"},
		"log-level":  "debug",
		"port":       8047,
		"profiles":   []string{"default"},
	}),
	expectedConfig: &config.Config{
		ImageName: "myimage",
		JujuAddrs: []string{"jimm.jujucharms.com:443"},
		LogLevel:  zapcore.DebugLevel,
		Port:      8047,
		Profiles:  []string{"default"},
	},
}, {
	about: "valid let's encrypt config",
	content: mustMarshalYAML(map[string]interface{}{
		"dns-name":   "shell.example.com",
		"image-name": "myimage",
		"juju-addrs": []string{"1.2.3.4", "4.3.2.1"},
		"log-level":  "debug",
		"port":       443,
		"profiles":   []string{"default", "termserver"},
	}),
	expectedConfig: &config.Config{
		DNSName:   "shell.example.com",
		ImageName: "myimage",
		JujuAddrs: []string{"1.2.3.4", "4.3.2.1"},
		LogLevel:  zapcore.DebugLevel,
		Port:      443,
		Profiles:  []string{"default", "termserver"},
	},
}, {
	about:         "unreadable config",
	content:       []byte("not a yaml"),
	expectedError: `cannot parse ".*": yaml: unmarshal errors:\n.*`,
}, {
	about:         "invalid config: missing fields",
	expectedError: `invalid configuration at ".*": missing fields: image-name, juju-addrs, port, profiles`,
}, {
	about: "invalid config: bad session timeout",
	content: mustMarshalYAML(map[string]interface{}{
		"image-name":      "myimage",
		"juju-addrs":      []string{"1.2.3.4", "4.3.2.1"},
		"port":            8047,
		"profiles":        []string{"default", "termserver"},
		"session-timeout": -1,
	}),
	expectedError: `invalid configuration at ".*": cannot specify a negative session timeout`,
}, {
	about: "invalid config for let's encrypt: keys specified",
	content: mustMarshalYAML(map[string]interface{}{
		"dns-name":   "shell.example.com",
		"image-name": "myimage",
		"juju-addrs": []string{"1.2.3.4", "4.3.2.1"},
		"log-level":  "debug",
		"port":       443,
		"profiles":   []string{"default", "termserver"},
		"tls-cert":   "TLS cert",
		"tls-key":    "TLS key",
	}),
	expectedError: `invalid configuration at ".*": cannot specify both DNS name for Let's Encrypt and TLS keys at the same time`,
}, {
	about: "invalid config for let's encrypt: bad port",
	content: mustMarshalYAML(map[string]interface{}{
		"dns-name":   "shell.example.com",
		"image-name": "myimage",
		"juju-addrs": []string{"1.2.3.4", "4.3.2.1"},
		"log-level":  "debug",
		"port":       4247,
		"profiles":   []string{"default", "termserver"},
	}),
	expectedError: `invalid configuration at ".*": cannot use a port different than 443 with Let's Encrypt`,
}}

func TestRead(t *testing.T) {
	for _, test := range readTests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)

			// Create the config file.
			f, err := ioutil.TempFile("", "config")
			c.Assert(err, qt.Equals, nil)
			defer f.Close()
			defer os.Remove(f.Name())
			_, err = f.Write(test.content)
			c.Assert(err, qt.Equals, nil)

			// Read the config file.
			conf, err := config.Read(f.Name())
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(conf, qt.IsNil)
				return
			}
			c.Assert(err, qt.Equals, nil)
			c.Assert(conf, qt.DeepEquals, test.expectedConfig)
		})
	}
}

func mustMarshalYAML(v interface{}) []byte {
	b, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
