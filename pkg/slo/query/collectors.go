/*
Module for defining collectors and their good/bad events API.

There are registered manually in preconfigured metrics builder in `def.go`

There are registered automatically to an auto-instrumentation server
in `pkg/test/instrumentation.go`
*/
package query

import (
	"fmt"
	"math/rand"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	uptimeCollector *prometheus.GaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "uptime_good",
	}, []string{"hostname", "ip", "job"})

	uptimeGoodEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		uptimeCollector.WithLabelValues(r.Host, r.RemoteAddr, MockTestServerName).Set(1)
	}
	uptimeBadEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		uptimeCollector.WithLabelValues(r.Host, r.RemoteAddr, MockTestServerName).Set(0)
	}

	availabilityCollector *prometheus.CounterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_request_duration_seconds_count",
	},
		[]string{"code", "job"},
	)
	availabilityGoodEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		randomStatusInt1 := rand.Intn(199)

		// anything between 200-399, and yes http status codes dont'work like this
		randomStatusCode := 200 + randomStatusInt1

		availabilityCollector.WithLabelValues(fmt.Sprintf("%d", randomStatusCode), MockTestServerName).Inc()
	}

	availabilityBadEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		randomStatusInt1 := rand.Intn(199)
		// anything between 400-599, and yes http status codes dont'work like this
		randomStatusCode := 400 + randomStatusInt1
		availabilityCollector.WithLabelValues(fmt.Sprintf("%d", randomStatusCode), MockTestServerName).Inc()
	}

	latencyCollector *prometheus.HistogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds_bucket",
	}, []string{"hostname", "ip", "job"})

	latencyGoodEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		// request duration faster than 0.3s / 0.300 ms
		randLatency := rand.Float64() * 0.29
		latencyCollector.WithLabelValues(r.Host, r.RemoteAddr, MockTestServerName).Observe(randLatency)
	}

	latencyBadEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		// request duration slower than 0.3s / 0.300 ms
		randLatency := rand.Float64() + 0.30
		latencyCollector.WithLabelValues(r.Host, r.RemoteAddr, MockTestServerName).Observe(randLatency)
	}
)
