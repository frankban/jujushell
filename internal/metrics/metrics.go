// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/CanonicalLtd/jujushell/internal/lxdclient"
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

// connection implements wstransport.Conn by wrapping the given WebSocket
// connection and adding metrics.
type connection struct {
	wstransport.Conn
	errorsCount *prometheus.CounterVec
}

// Error implements wstransport.Conn.Error.
func (conn *connection) Error(err error) error {
	err = conn.Conn.Error(err)
	conn.errorsCount.WithLabelValues(err.Error()).Inc()
	return err
}

// InstrumentLXDClient is a wrapper for lxdclient.Client which observes the
// duration of common client actions, like creating or retreiving containers.
func InstrumentLXDClient(client lxdclient.Client) lxdclient.Client {
	inFlight := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "containers_in_flight",
		Help:      "the number of containers currently present in the machine",
	})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "containers_duration",
		Help:      "time spent doing container operations",
		Buckets:   []float64{.25, .5, 1, 1.5, 2, 3, 5, 10},
	}, []string{"operation"})
	return &lxdClient{
		Client:   client,
		inFlight: mustRegisterOnce(inFlight).(prometheus.Gauge),
		duration: mustRegisterOnce(duration).(*prometheus.HistogramVec),
	}
}

// lxdClient implements lxdclient.Client by wrapping the given LXD client and
// adding metrics.
type lxdClient struct {
	lxdclient.Client
	inFlight prometheus.Gauge
	duration *prometheus.HistogramVec
}

// All implements lxdclient.Client.All.
func (client *lxdClient) All() ([]lxdclient.Container, error) {
	observe := timeit(client.duration.WithLabelValues("get-all-containers"))
	defer observe()
	cs, err := client.Client.All()
	if err == nil {
		client.inFlight.Set(float64(len(cs)))
	}
	return cs, err
}

// Create implements lxdclient.Client.Create.
func (client *lxdClient) Create(image, name string, profiles ...string) (lxdclient.Container, error) {
	observe := timeit(client.duration.WithLabelValues("create-container"))
	defer observe()
	return client.Client.Create(image, name, profiles...)
}

// Delete implements lxdclient.Client.Delete.
func (client *lxdClient) Delete(name string) error {
	observe := timeit(client.duration.WithLabelValues("delete-container"))
	defer observe()
	return client.Client.Delete(name)
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

func timeit(observer prometheus.Observer) func() {
	start := time.Now()
	return func() {
		observer.Observe(time.Since(start).Seconds())
	}
}
