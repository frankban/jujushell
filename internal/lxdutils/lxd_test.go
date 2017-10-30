// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdutils_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	macaroon "gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jujushell/internal/juju"
	"github.com/CanonicalLtd/jujushell/internal/lxdutils"
)

var internalEnsureTests = []struct {
	about string
	srv   *srv
	image string
	info  *juju.Info
	creds *juju.Credentials

	expectedAddr  string
	expectedError string

	expectedCreateReq       api.ContainersPost
	expectedUpdateReqs      []api.ContainerStatePut
	expectedUpdateName      string
	expectedDeleteName      string
	expectedGetStateName    string
	expectedGetFileName     string
	expectedGetFilePaths    []string
	expectedCreateFileName  string
	expectedCreateFilePaths []string
	expectedCreateFileArgs  []lxd.ContainerFileArgs
	expectedSleepCalls      int
	expectedExecName        string
	expectedExecReqs        []api.ContainerExecPost
	expectedExecArgs        []*lxd.ContainerExecArgs
}{{
	about: "error getting containers",
	srv: &srv{
		getError: errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: "cannot get containers: bad wolf",
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedDeleteName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error creating the container",
	srv: &srv{
		createError: errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: `cannot create container "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedDeleteName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error in the operation of creating the container",
	srv: &srv{
		createOpError: errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "rose",
	},
	expectedError: "create container operation failed: bad wolf",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedDeleteName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
}, {
	about: "error stopping the container",
	srv: &srv{
		createError: errors.New("bad wolf"),
		stopError:   errors.New("stop error"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: `cannot create container "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error in the operation of stopping the container",
	srv: &srv{
		createError: errors.New("bad wolf"),
		stopOpError: errors.New("stop op error"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: `cannot create container "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error starting the container",
	srv: &srv{
		startError: errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: `cannot start container "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedDeleteName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error in the operation of starting the container",
	srv: &srv{
		startOpError: errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "rose",
	},
	expectedError: "start container operation failed: bad wolf",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedDeleteName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
}, {
	about: "error retrieving container state",
	srv: &srv{
		getStateError: errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: `cannot get state for container "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedDeleteName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
}, {
	about: "error retrieving container address",
	srv:   &srv{},
	image: "termserver",
	info: &juju.Info{
		User: "who",
	},
	expectedError: `cannot find address for "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who"`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedDeleteName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedSleepCalls:   300,
}, {
	about: "error setting up the macaroons cookie jar",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
	},
	image: "termserver",
	info: &juju.Info{
		User: "dalek@external",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{},
		},
	},
	expectedError: `cannot set macaroons in jar: cannot create cookie for "https://1.2.3.4/identity": no macaroons in cookie`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
}, {
	about: "error getting directory information from the container",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps: []fileResponse{{}, {}, {isFile: true}},
	},
	image: "termserver",
	info: &juju.Info{
		User: "dalek@external",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create cookie file in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": cannot create directory "/home/ubuntu/.local": a file with the same name exists in the container`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local",
	},
}, {
	about: "error creating the path for the cookie file",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {hasErr: true}},
		createFileErrors: []error{errors.New("bad wolf")},
	},
	image: "termserver",
	info: &juju.Info{
		User: "dalek@external",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create cookie file in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": cannot create directory "/home/ubuntu/.local/share/juju" in the container: bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		UID:  42,
		GID:  47,
		Mode: 0700,
		Type: "directory",
	}},
}, {
	about: "error creating the cookie file",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {hasErr: true}},
		createFileErrors: []error{errors.New("bad wolf")},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "dalek@external",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create cookie file in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": cannot create file "/home/ubuntu/.local/share/juju/cookies/my-controller.json" in the container: bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
	},
	expectedCreateFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/my-controller.json"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
}, {
	about: "error creating the path for the controllers file",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {hasErr: true}},
		createFileErrors: []error{nil, errors.New("bad wolf")},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "dalek@external",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create controllers file in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": cannot create directory "/home/ubuntu/.local/share" in the container: bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share",
	},
	expectedCreateFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/my-controller.json", "/home/ubuntu/.local/share"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		UID:  42,
		GID:  47,
		Mode: 0700,
		Type: "directory",
	}},
}, {
	about: "error creating the controller file",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {hasErr: true}},
		createFileErrors: []error{nil, errors.New("bad wolf")},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "dalek@external",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot create controllers file in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": cannot create file "/home/ubuntu/.local/share/juju/controllers.yaml" in the container: bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/my-controller.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
}, {
	about: "error logging into juju in the container",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
		execError:        errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User:           "dalek@external",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot log into Juju in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": cannot execute command "su - ubuntu -c juju login -c my-controller": bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/my-controller.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c my-controller"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "error in the operation of logging into juju in the container",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
		execOpError:      errors.New("bad wolf"),
	},
	image: "termserver",
	info: &juju.Info{
		User:           "dalek@external",
		ControllerName: "my-controller",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedError: `cannot log into Juju in container "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external": execute command operation failed: bad wolf`,
	expectedCreateReq: api.ContainersPost{
		Name: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}, {
		Action:  "stop",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedDeleteName:   "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetStateName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/my-controller.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-ba5d6ad35b5468ef1990ea04f8a81503605d6b79-dalek-external",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c my-controller"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet6",
			Scope:   "global",
		}, {
			Address: "1.2.3.5",
			Family:  "inet",
			Scope:   "local",
		}, {
			Address: "1.2.3.6",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "rose",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr: "1.2.3.6",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetStateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetFileName:  "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/ctrl.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success with userpass authentication",
	srv: &srv{
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet6",
			Scope:   "global",
		}, {
			Address: "1.2.3.5",
			Family:  "inet",
			Scope:   "local",
		}, {
			Address: "1.2.3.6",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "rose",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Username: "who",
		Password: "secret",
	},
	expectedAddr: "1.2.3.6",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetStateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetFileName:  "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetFilePaths: []string{
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success with other containers around",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-dalek",
			Status: "Stopped",
		}, {
			Name:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-cyberman",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet6",
			Scope:   "global",
		}, {
			Address: "1.2.3.5",
			Family:  "inet",
			Scope:   "local",
		}, {
			Address: "1.2.3.6",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "rose",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr: "1.2.3.6",
	expectedCreateReq: api.ContainersPost{
		Name: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
		Source: api.ContainerSource{
			Type:  "image",
			Alias: "termserver",
		},
	},
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetStateName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetFileName:  "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/ctrl.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-rose",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success with container already created",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-7b7074fca36fc89fb3f1e3c46d74f6ffe2477a09-dalek",
			Status: "Stopped",
		}, {
			Name:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
			Status: "Stopped",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "who",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr: "1.2.3.4",
	expectedUpdateReqs: []api.ContainerStatePut{{
		Action:  "start",
		Timeout: -1,
	}},
	expectedUpdateName:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetFileName:  "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/ctrl.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success with container already started",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "who",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr:         "1.2.3.4",
	expectedGetStateName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetFileName:  "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/ctrl.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-b7adf77905f540249517ca164255899e9ad1e2ac-who",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success with container already started (external user)",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "cyberman@external",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr:         "1.2.3.4",
	expectedGetStateName: "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
	expectedGetFileName:  "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/ctrl.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-fc1565bb1f8fe145fda53955901546405e01a80b-cyberman-externa",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}, {
	about: "success with container already started (user with special chars)",
	srv: &srv{
		containers: []api.Container{{
			Name:   "ts-f9d4707cdb10d7be9e7936b88d8e6ed4998edd2d-these-are-the--v",
			Status: "Started",
		}},
		getStateAddresses: []api.ContainerStateNetworkAddress{{
			Address: "1.2.3.4",
			Family:  "inet",
			Scope:   "global",
		}},
		getFileResps:     []fileResponse{{}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}},
		createFileErrors: []error{nil, nil},
	},
	image: "termserver",
	info: &juju.Info{
		User:           "these.are@the++voy_age",
		ControllerName: "ctrl",
	},
	creds: &juju.Credentials{
		Macaroons: map[string]macaroon.Slice{
			"https://1.2.3.4/identity": macaroon.Slice{mustNewMacaroon("m1")},
		},
	},
	expectedAddr:         "1.2.3.4",
	expectedGetStateName: "ts-f9d4707cdb10d7be9e7936b88d8e6ed4998edd2d-these-are-the--v",
	expectedGetFileName:  "ts-f9d4707cdb10d7be9e7936b88d8e6ed4998edd2d-these-are-the--v",
	expectedGetFilePaths: []string{
		// Path to the cookie file dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju", "/home/ubuntu/.local/share/juju/cookies",
		// Path to the controllers.yaml dir.
		"/home", "/home/ubuntu", "/home/ubuntu/.local", "/home/ubuntu/.local/share", "/home/ubuntu/.local/share/juju",
	},
	expectedCreateFileName:  "ts-f9d4707cdb10d7be9e7936b88d8e6ed4998edd2d-these-are-the--v",
	expectedCreateFilePaths: []string{"/home/ubuntu/.local/share/juju/cookies/ctrl.json", "/home/ubuntu/.local/share/juju/controllers.yaml"},
	expectedCreateFileArgs: []lxd.ContainerFileArgs{{
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}, {
		Content: strings.NewReader("thi is just a placeholder: see createFileReqComparer below"),
		UID:     42,
		GID:     47,
		Mode:    0600,
	}},
	expectedExecName: "ts-f9d4707cdb10d7be9e7936b88d8e6ed4998edd2d-these-are-the--v",
	expectedExecReqs: []api.ContainerExecPost{{
		Command:   []string{"su", "-", "ubuntu", "-c", "juju login -c ctrl"},
		WaitForWS: true,
	}},
	expectedExecArgs: []*lxd.ContainerExecArgs{{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}},
}}

func TestEnsure(t *testing.T) {
	createFileReqComparer := func(a, b io.ReadSeeker) bool {
		return (a != nil && b != nil) || a == b
	}
	for _, test := range internalEnsureTests {
		t.Run(test.about, func(t *testing.T) {
			c := qt.New(t)
			s := &sleeper{
				c: c,
			}
			restore := patchSleep(s.sleep)
			defer restore()
			addr, err := lxdutils.Ensure(test.srv, test.image, test.info, test.creds)
			if test.expectedError != "" {
				c.Assert(err, qt.ErrorMatches, test.expectedError)
				c.Assert(addr, qt.Equals, "")
			} else {
				c.Assert(err, qt.Equals, nil)
				c.Assert(addr, qt.Equals, test.expectedAddr)
			}
			c.Assert(test.srv.createReq, qt.DeepEquals, test.expectedCreateReq)
			c.Assert(test.srv.updateReqs, qt.DeepEquals, test.expectedUpdateReqs)
			c.Assert(test.srv.updateName, qt.Equals, test.expectedUpdateName)
			c.Assert(test.srv.deleteName, qt.Equals, test.expectedDeleteName)
			c.Assert(test.srv.getStateName, qt.Equals, test.expectedGetStateName)
			c.Assert(test.srv.getFileName, qt.Equals, test.expectedGetFileName)
			c.Assert(test.srv.getFilePaths, qt.DeepEquals, test.expectedGetFilePaths)
			c.Assert(test.srv.createFileName, qt.Equals, test.expectedCreateFileName)
			c.Assert(test.srv.createFilePaths, qt.DeepEquals, test.expectedCreateFilePaths)
			c.Assert(test.srv.createFileArgs, qt.CmpEquals(cmp.Comparer(createFileReqComparer)), test.expectedCreateFileArgs)
			c.Assert(test.srv.execName, qt.Equals, test.expectedExecName)
			c.Assert(test.srv.execReqs, qt.DeepEquals, test.expectedExecReqs)
			c.Assert(test.srv.execArgs, qt.CmpEquals(cmpopts.IgnoreUnexported(*os.Stdin)), test.expectedExecArgs)
			c.Assert(s.callCount, qt.Equals, test.expectedSleepCalls)
		})
	}
}

// srv implements lxd.ContainerServer for testing purposes.
type srv struct {
	lxd.ContainerServer

	containers []api.Container
	getError   error

	createReq     api.ContainersPost
	createError   error
	createOpError error

	updateName   string
	updateReqs   []api.ContainerStatePut
	startError   error
	startOpError error
	stopError    error
	stopOpError  error

	getStateName      string
	getStateAddresses []api.ContainerStateNetworkAddress
	getStateError     error

	deleteName string

	getFileName  string
	getFilePaths []string
	getFileResps []fileResponse

	createFileName   string
	createFilePaths  []string
	createFileArgs   []lxd.ContainerFileArgs
	createFileErrors []error

	execName    string
	execReqs    []api.ContainerExecPost
	execArgs    []*lxd.ContainerExecArgs
	execError   error
	execOpError error
}

func (s *srv) GetContainers() ([]api.Container, error) {
	return s.containers, s.getError
}

func (s *srv) CreateContainer(req api.ContainersPost) (*lxd.Operation, error) {
	s.createReq = req
	if s.createError != nil {
		return nil, s.createError
	}
	return newOp(s.createOpError), nil
}

func (s *srv) UpdateContainerState(name string, req api.ContainerStatePut, ETag string) (*lxd.Operation, error) {
	if s.updateName == "" {
		s.updateName = name
	}
	if s.updateName != name {
		panic("updating two different containers in the same request")
	}
	s.updateReqs = append(s.updateReqs, req)
	var err, opErr error
	switch req.Action {
	case "start":
		err = s.startError
		opErr = s.startOpError
	case "stop":
		err = s.stopError
		opErr = s.stopOpError
	default:
		panic("unexpected action " + req.Action)
	}
	if err != nil {
		return nil, err
	}
	return newOp(opErr), nil
}

func (s *srv) GetContainerState(name string) (*api.ContainerState, string, error) {
	s.getStateName = name
	if s.getStateError != nil {
		return nil, "", s.getStateError
	}
	return &api.ContainerState{
		Network: map[string]api.ContainerStateNetwork{
			"eth0": {
				Addresses: s.getStateAddresses,
			},
		},
	}, "", nil
}

func (s *srv) GetContainerFile(name, path string) (io.ReadCloser, *lxd.ContainerFileResponse, error) {
	if len(s.getFileResps) == 0 {
		panic("cannot serve file " + path)
	}
	if s.getFileName == "" {
		s.getFileName = name
	}
	if s.getFileName != name {
		panic("getting files from two different containers in the same request")
	}
	s.getFilePaths = append(s.getFilePaths, path)
	resp := s.getFileResps[0]
	s.getFileResps = s.getFileResps[1:]
	return resp.value()
}

func (s *srv) CreateContainerFile(name, path string, args lxd.ContainerFileArgs) error {
	if len(s.createFileErrors) == 0 {
		panic("cannot create file " + path)
	}
	if s.createFileName == "" {
		s.createFileName = name
	}
	if s.createFileName != name {
		panic("creating files from two different containers in the same request")
	}
	s.createFilePaths = append(s.createFilePaths, path)
	s.createFileArgs = append(s.createFileArgs, args)
	err := s.createFileErrors[0]
	s.createFileErrors = s.createFileErrors[1:]
	return err
}

func (s *srv) ExecContainer(name string, req api.ContainerExecPost, args *lxd.ContainerExecArgs) (*lxd.Operation, error) {
	if s.execName == "" {
		s.execName = name
	}
	if s.execName != name {
		panic("executing commands in two different containers in the same request")
	}
	s.execReqs = append(s.execReqs, req)
	s.execArgs = append(s.execArgs, args)
	if s.execError != nil {
		return nil, s.execError
	}
	return newOp(s.execOpError), nil
}

func (s *srv) DeleteContainer(name string) (*lxd.Operation, error) {
	s.deleteName = name
	return newOp(nil), nil
}

// newOp creates and return a new LXD operation whose Wait method returns the
// provided error.
func newOp(err error) *lxd.Operation {
	op := lxd.Operation{
		Operation: api.Operation{
			StatusCode: api.Success,
		},
	}
	if err != nil {
		op.StatusCode = api.Failure
		op.Err = err.Error()
	}
	return &op
}

// fileResponse is used to build responses to
// lxd.ContainerServer.CreateContainerFile calls.
type fileResponse struct {
	isFile bool
	hasErr bool
}

func (r fileResponse) value() (io.ReadCloser, *lxd.ContainerFileResponse, error) {
	if r.hasErr {
		return nil, nil, errors.New("no such file")
	}
	resp := &lxd.ContainerFileResponse{
		UID:  42,
		GID:  47,
		Type: "directory",
	}
	if r.isFile {
		resp.Type = "file"
	}
	return nil, resp, nil
}

// sleeper is used to patch time.Sleep.
type sleeper struct {
	c         *qt.C
	callCount int
}

func (s *sleeper) sleep(d time.Duration) {
	s.callCount++
	s.c.Assert(d, qt.Equals, 100*time.Millisecond)
}

// patchSleep patches the api.sleep variable so that it is possible to avoid
// sleeping in tests.
func patchSleep(f func(d time.Duration)) (restore func()) {
	original := *lxdutils.Sleep
	*lxdutils.Sleep = f
	return func() {
		*lxdutils.Sleep = original
	}
}

func mustNewMacaroon(root string) *macaroon.Macaroon {
	m, err := macaroon.New([]byte(root), "id", "loc")
	if err != nil {
		panic(err)
	}
	return m
}
