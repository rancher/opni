package extensions

/*
Contains the AlertManager Opni embedded server implementation.
The embedded service must be run within the same process as each
deploymed node in the AlertManager cluster.
*/

import (
	"context"
	"errors"
	"net/http"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"

	// add profiles
	_ "net/http/pprof"
)

const (
	missingTitle = "missing title"
	missingBody  = "missing body"
)

var defaultSeverity = alertingv1.OpniSeverity_Info.String()

func truncateMessageContent(content string) string {
	if len(content) > 1000 {
		content = content[:1000] + "<truncated>"
	}
	return content
}

type messageMetadata struct {
	isAlarm           bool
	uuid              string
	groupDedupeKey    string
	fingerprint       string
	sourceFingerprint string
	severity          int32
}

// Embedded Server handles all incoming webhook requests from the AlertManager
type EmbeddedServer struct {
	logger *zap.SugaredLogger
	// maxSize of the combined caches
	lub int
	// layered caches
	notificationCache messageCache[alertingv1.OpniSeverity, *alertingv1.MessageInstance]
	alarmCache        messageCache[alertingv1.OpniSeverity, *alertingv1.MessageInstance]
}

func NewEmbeddedServer(
	lg *zap.SugaredLogger,
	lub int,
) *EmbeddedServer {
	return &EmbeddedServer{
		logger:            lg,
		lub:               lub,
		notificationCache: NewLFUMessageCache(lub),
		alarmCache:        NewLFUMessageCache(lub),
	}
}

func StartOpniEmbeddedServer(ctx context.Context, opniAddr string) *http.Server {
	lg := logger.NewPluginLogger().Named("opni.alerting")
	es := NewEmbeddedServer(lg, 125)
	mux := http.NewServeMux()

	// request body will be in the form of AM webhook payload :
	// https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
	//
	// Note :
	//    Webhooks are assumed to respond with 2xx response codes on a successful
	//	  request and 5xx response codes are assumed to be recoverable.
	// therefore, non-recoverable errors should have error codes 3XX and 4XX
	mux.HandleFunc(shared.AlertingDefaultHookName, es.handleWebhook)
	mux.HandleFunc("/notifications/list", es.handleListNotifications)
	mux.HandleFunc("/alarms/list", es.handleListAlarms)

	hookServer := &http.Server{
		// explicitly set this to 0.0.0.0 for test environment
		Addr:    opniAddr,
		Handler: mux,
	}
	go func() {
		lg.With("addr", opniAddr).Info("starting opni embedded server")
		err := hookServer.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := hookServer.Close(); err != nil {
			panic(err)
		}
	}()
	return hookServer
}
