// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics_test

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/juju/jujushell/internal/lxdclient"
	"github.com/juju/jujushell/internal/metrics"
	"github.com/juju/jujushell/internal/wstransport"
)

func TestInstrumentHandler(t *testing.T) {
	c := qt.New(t)
	codes := []int{
		http.StatusOK,
		http.StatusOK,
		http.StatusInternalServerError,
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusOK,
		http.StatusBadRequest,
	}
	// Set up a simple instrumented server.
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		code := codes[0]
		codes = codes[1:]
		w.WriteHeader(code)
	})
	iSrv := httptest.NewServer(metrics.InstrumentHandler(handler))
	defer iSrv.Close()

	// Set up a metrics server.
	metricsSrv := httptest.NewServer(promhttp.Handler())
	defer metricsSrv.Close()

	// Connect to the instrumented server multiple times.
	for range codes {
		_, err := http.DefaultClient.Get(iSrv.URL)
		c.Assert(err, qt.Equals, nil)
	}

	// Check the resulting metrics (just the counts as they are deterministic).
	checkMetrics(c, metricsSrv.URL, "jujushell_requests_count", []string{
		"# HELP jujushell_requests_count the total count of requests",
		"# TYPE jujushell_requests_count counter",
		`jujushell_requests_count{code="200"} 4`,
		`jujushell_requests_count{code="400"} 2`,
		`jujushell_requests_count{code="500"} 1`,
	})
}

func TestInstrumentWSConnection(t *testing.T) {
	c := qt.New(t)
	errs := []string{"bad wolf", "bad wolf", "exterminate"}
	// Set up a WebSocket server that writes a JSON error response.
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := wstransport.Upgrade(w, req)
		c.Assert(err, qt.Equals, nil)
		defer conn.Close()
		conn = metrics.InstrumentWSConnection(conn)
		msg := errs[0]
		errs = errs[1:]
		conn.Error(errors.New(msg))
	}))
	defer wsSrv.Close()

	// Set up a metrics server.
	metricsSrv := httptest.NewServer(promhttp.Handler())
	defer metricsSrv.Close()

	// Connect to the WebSocket server multiple times.
	url := wsURL(wsSrv.URL)
	for range errs {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		c.Assert(err, qt.Equals, nil)
		conn.Close()
	}

	// Check the resulting metrics .
	checkMetrics(c, metricsSrv.URL, "jujushell_errors", []string{
		"# HELP jujushell_errors_count the number of encountered errors",
		"# TYPE jujushell_errors_count counter",
		`jujushell_errors_count{message="bad wolf"} 2`,
		`jujushell_errors_count{message="exterminate"} 1`,
	})
}

func TestInstrumentLXDClient(t *testing.T) {
	c := qt.New(t)
	var cl lxdclient.Client = &client{}
	cl = metrics.InstrumentLXDClient(cl)

	// Set up a metrics server.
	metricsSrv := httptest.NewServer(promhttp.Handler())
	defer metricsSrv.Close()

	// Work with the client.
	cl.Create("image", "name")
	cl.Create("image", "name")
	cl.All()

	// Check the resulting metrics (just the counts as they are deterministic).
	checkMetrics(c, metricsSrv.URL, "jujushell_containers_duration_count", []string{
		`jujushell_containers_duration_count{operation="create-container"} 2`,
		`jujushell_containers_duration_count{operation="get-all-containers"} 1`,
	})
	checkMetrics(c, metricsSrv.URL, "jujushell_containers_in_flight", []string{
		"# HELP jujushell_containers_in_flight the number of containers currently present in the machine",
		"# TYPE jujushell_containers_in_flight gauge",
		"jujushell_containers_in_flight 2",
	})

	// Work more.
	cl.Delete("name")
	cl.Create("image", "name")
	cl.Create("image", "name")
	cl.All()

	// Check the resulting metrics again.
	checkMetrics(c, metricsSrv.URL, "jujushell_containers_duration_count", []string{
		`jujushell_containers_duration_count{operation="create-container"} 4`,
		`jujushell_containers_duration_count{operation="delete-container"} 1`,
		`jujushell_containers_duration_count{operation="get-all-containers"} 2`,
	})
	checkMetrics(c, metricsSrv.URL, "jujushell_containers_in_flight", []string{
		"# HELP jujushell_containers_in_flight the number of containers currently present in the machine",
		"# TYPE jujushell_containers_in_flight gauge",
		"jujushell_containers_in_flight 3",
	})
}

// client implements lxdclient.Client for testing purposes.
type client struct {
	lxdclient.Client
	numContainer int
}

func (cl *client) All() ([]lxdclient.Container, error) {
	return make([]lxdclient.Container, cl.numContainer), nil
}

func (cl *client) Create(image, name string, profiles ...string) (lxdclient.Container, error) {
	cl.numContainer++
	return nil, nil
}

func (cl *client) Delete(name string) error {
	cl.numContainer--
	return nil
}

func checkMetrics(c *qt.C, url, substr string, expectedLines []string) {
	timeout := time.After(5 * time.Second)
	tick := time.Tick(100 * time.Millisecond)
	lines := make([]string, 0, len(expectedLines))
	getLines := func() []string {
		resp, err := http.DefaultClient.Get(url)
		c.Assert(err, qt.Equals, nil)
		defer resp.Body.Close()
		return linesContaining(c, resp.Body, substr)
	}
loop:
	for {
		select {
		case <-timeout:
			break loop
		case <-tick:
			lines = getLines()
			if len(lines) == len(expectedLines) {
				break loop
			}
		}
	}
	c.Assert(lines, qt.DeepEquals, expectedLines)
}

// wsURL returns a WebSocket URL from the given HTTP URL.
func wsURL(u string) string {
	return strings.Replace(u, "http://", "ws://", 1)
}

// linesContaining returns the lines from the given reader that contain the
// given substr.
func linesContaining(c *qt.C, r io.Reader, substr string) (lines []string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, substr) {
			lines = append(lines, line)
		}
	}
	c.Assert(scanner.Err(), qt.Equals, nil)
	return lines
}
