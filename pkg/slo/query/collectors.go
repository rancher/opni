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

var verbs = []string{"POST", "GET", "PUT", "DELETE"}
var goodCodes = []int{200, 201, 202}
var badCodes = []int{404, 429, 500, 502, 503}

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
		[]string{"code", "verb"},
	)
	availabilityGoodEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		randomStatusCode := goodCodes[rand.Intn(len(goodCodes))]
		randomVerb := verbs[rand.Intn(len(verbs))]

		availabilityCollector.WithLabelValues(fmt.Sprintf("%d", randomStatusCode), randomVerb).Inc()
	}

	availabilityBadEvents http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		randomStatusCode := badCodes[rand.Intn(len(badCodes))]
		randomVerb := verbs[rand.Intn(len(verbs))]
		availabilityCollector.WithLabelValues(fmt.Sprintf("%d", randomStatusCode), randomVerb).Inc()
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
