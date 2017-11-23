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

	qt "github.com/frankban/quicktest"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/CanonicalLtd/jujushell/internal/metrics"
	"github.com/CanonicalLtd/jujushell/internal/wstransport"
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

	// Check the resulting metrics.
	checkMetrics(c, metricsSrv.URL, "jujushell_errors", []string{
		"# HELP jujushell_errors_count the number of encountered errors",
		"# TYPE jujushell_errors_count counter",
		`jujushell_errors_count{message="bad wolf"} 2`,
		`jujushell_errors_count{message="exterminate"} 1`,
	})
}

func checkMetrics(c *qt.C, url, substr string, expectedLines []string) {
	resp, err := http.DefaultClient.Get(url)
	c.Assert(err, qt.Equals, nil)
	defer resp.Body.Close()
	lines := linesContaining(c, resp.Body, substr)
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
