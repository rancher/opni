package extensions

import (
	"errors"
	"net/http"

	"github.com/rancher/opni/pkg/alerting/shared"
	// "github.com/rancher/opni/pkg/logger"
)

/*
Contains the AlertManager Opni embedded server implementation.
The embedded service must be run within the same process as each
deploymed node in the AlertManager cluster.
*/

func StartOpniEmbeddedServer(opniAddr string) *http.Server {
	// lg := logger.NewPluginLogger().Named("opni.alerting")
	mux := http.NewServeMux()
	// request body will be in the form of AM webhook payload :
	// https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
	//
	// Note :
	//    Webhooks are assumed to respond with 2xx response codes on a successful
	//	  request and 5xx response codes are assumed to be recoverable.
	// therefore, non-recoverable errors should have error codes 3XX and 4XX
	mux.HandleFunc(shared.AlertingDefaultHookName, func(wr http.ResponseWriter, req *http.Request) {})

	hookServer := &http.Server{
		// explicitly set this to 0.0.0.0 for test environment
		Addr:    opniAddr,
		Handler: mux,
	}
	go func() {
		err := hookServer.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	return hookServer
}
