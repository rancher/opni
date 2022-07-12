package test

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/phayes/freeport"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rancher/opni/pkg/slo/query"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
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

	// create handlers for each metric / status pair
	grpcEnum := sloapi.SLOStatusState_name
	for queryName := range query.AvailableQueries {
		for _, enumName := range grpcEnum {
			if strings.Contains(enumName, "InternalError") || strings.Contains(enumName, "NoData") {
				continue
			}
			mux.HandleFunc(fmt.Sprintf("%s/%s", queryName, enumName), noFunc)
		}
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
		if err != nil {
			panic(err)
		}
	}()
	done := make(chan bool)
	go func() {
		select {
		case <-done:
			err := autoInstrumentationServer.Shutdown(nil)
			panic(err)
		}
	}()

	return port, done
}
