// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"

	"github.com/juju/jujushell/apiparams"
	"github.com/juju/jujushell/internal/api"
)

func TestWaitReady(t *testing.T) {
	c := qt.New(t)
	tests := []struct {
		about              string
		handler            http.Handler
		expectedSleepCalls int
		expectedError      string
	}{{
		about: "immediate success",
		handler: handler(c, mustMarshalJSON(apiparams.Response{
			Code: apiparams.OK,
		}), 0),
	}, {
		about: "immediate failure",
		handler: handler(c, mustMarshalJSON(apiparams.Response{
			Code: apiparams.Error,
		}), 0),
		expectedError: `invalid response from .*: "error"`,
	}, {
		about: "eventual success",
		handler: handler(c, mustMarshalJSON(apiparams.Response{
			Code: apiparams.OK,
		}), 2),
		expectedSleepCalls: 2,
	}, {
		about: "eventual failure",
		handler: handler(c, mustMarshalJSON(apiparams.Response{
			Code: apiparams.Error,
		}), 10),
		expectedSleepCalls: 10,
		expectedError:      `invalid response from .*: "error"`,
	}, {
		about: "failure for too many retries",
		handler: handler(c, mustMarshalJSON(apiparams.Response{
			Code: apiparams.OK,
		}), 1000),
		expectedSleepCalls: 100,
		expectedError:      "cannot get .*: EOF",
	}, {
		about:         "failure for non JSON response",
		handler:       handler(c, "bad wolf", 0),
		expectedError: "cannot decode response: invalid character .*",
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			withServer(c, test.handler, test.expectedSleepCalls, func(url string) {
				err := api.WaitReady(url)
				if test.expectedError != "" {
					c.Assert(err, qt.ErrorMatches, test.expectedError)
					return
				}
				c.Assert(err, qt.Equals, nil)
			})
		})
	}
}

// withServer runs the given function in the context of a test server, with
// time.Sleep opportunely patched.
func withServer(c *qt.C, handler http.Handler, expectedSleepCalls int, f func(url string)) {
	srv := httptest.NewServer(handler)
	defer srv.Close()
	s := &sleeper{
		c: c,
	}
	restore := patchSleep(s.sleep)
	defer restore()
	f(srv.URL)
	c.Assert(s.callCount, qt.Equals, expectedSleepCalls)
}

// handler returns an http.Handler that returns a response with the given body
// after the given number of failed attempts.
func handler(c *qt.C, body string, after int) http.Handler {
	var counter int
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if counter < after {
			counter++
			// Simulate a scenario in which the server is not available.
			runtime.Goexit()
		}
		fmt.Fprint(w, body)
	})
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
	original := *api.Sleep
	*api.Sleep = f
	return func() {
		*api.Sleep = original
	}
}

func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
