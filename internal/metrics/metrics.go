// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/CanonicalLtd/jujushell/internal/wstransport"
)

// namespace is used as a prefix for all jujushell related metrics.
const namespace = "jujushell"

// InstrumentHandler is a middleware that wraps the provided http.Handler to
// observe the request duration, count the total number of requests, and
// measure how many requests are currently in flight.
func InstrumentHandler(handler http.Handler) http.Handler {
	requestsInFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "requests_in_flight",
		Help:      "the number of requests currently in flight",
	})
	requestsInFlight = mustRegisterOnce(requestsInFlight).(prometheus.Gauge)

	requestsCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "requests_count",
		Help:      "the total count of requests",
	}, []string{"code"})
	requestsCount = mustRegisterOnce(requestsCount).(*prometheus.CounterVec)

	requestsDuration := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: namespace,
		Name:      "requests_duration",
		Help:      "time spent in requests",
	}, []string{"code"})
	requestsDuration = mustRegisterOnce(requestsDuration).(*prometheus.SummaryVec)

	return promhttp.InstrumentHandlerInFlight(
		requestsInFlight, promhttp.InstrumentHandlerCounter(
			requestsCount, promhttp.InstrumentHandlerDuration(
				requestsDuration, handler)))
}

// InstrumentWSConnection is a decorator for WebSocket connections. It observes
// the errors sent via the WebSocket.
func InstrumentWSConnection(conn wstransport.Conn) wstransport.Conn {
	errorsCount := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "errors_count",
		Help:      "the number of encountered errors",
	}, []string{"message"}) // We don't expecte many different messages.
	return &connection{
		Conn:        conn,
		errorsCount: mustRegisterOnce(errorsCount).(*prometheus.CounterVec),
	}
}

// connection is a wrapper for a WebSocket connection.
type connection struct {
	wstransport.Conn
	errorsCount *prometheus.CounterVec
}

// Error implements wstransport.Error by increasing the number of errors of any
// encountered type.
func (conn *connection) Error(err error) error {
	err = conn.Conn.Error(err)
	conn.errorsCount.WithLabelValues(err.Error()).Inc()
	return err
}

// mustRegisterOnce registers the given metrics collector only if not already
// registered. It returns the registered collector.
func mustRegisterOnce(c prometheus.Collector) prometheus.Collector {
	if err := prometheus.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			c = are.ExistingCollector
		} else {
			panic(err)
		}
	}
	return c
}
