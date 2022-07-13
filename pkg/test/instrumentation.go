package test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/phayes/freeport"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rancher/opni/pkg/slo/query"
)

func noFunc(w http.ResponseWriter, r *http.Request) {}

// Stars a server that exposes Prometheus metrics
//
// Returns port number of the server & a channel that shutdowns the server
func (e *Environment) StartInstrumentationServer() (int, chan bool) {
	// lg := e.Logger
	port, err := freeport.GetFreePort()
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()

	for queryName, queryObj := range query.AvailableQueries {
		// register each prometheus collector
		prometheus.MustRegister(queryObj.GetCollector())

		// create an endpoint simulating good events
		mux.HandleFunc(fmt.Sprintf("/%s/%s", queryName, "good"), queryObj.GetGoodEventGenerator())
		// create an endpoint simulating bad events
		mux.HandleFunc(fmt.Sprintf("/%s/%s", queryName, "bad"), queryObj.GetBadEventGenerator())

	}
	// expose prometheus metrics
	mux.Handle("/metrics", promhttp.Handler())

	autoInstrumentationServer := &http.Server{
		Addr:           fmt.Sprintf("127.0.0.1:%d", port),
		Handler:        mux,
		ReadTimeout:    1 * time.Second,
		WriteTimeout:   1 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		err := autoInstrumentationServer.ListenAndServe()
		if err != http.ErrServerClosed {
			panic(err)
		}
	}()
	done := make(chan bool)
	go func() {
		select {
		case <-done:
			autoInstrumentationServer.Shutdown(context.Background())
		}
	}()

	return port, done
}
