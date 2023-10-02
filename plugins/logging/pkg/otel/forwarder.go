package otel

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Forwarder struct {
	*LogsForwarder
	*TraceForwarder
}

func NewForwarder(logsForwarder *LogsForwarder, traceForwarder *TraceForwarder) *Forwarder {
	return &Forwarder{
		LogsForwarder:  logsForwarder,
		TraceForwarder: traceForwarder,
	}
}

type forwarderOptions struct {
	collectorAddressOverride string
	cc                       grpc.ClientConnInterface
	lg                       *zap.SugaredLogger
	dialOptions              []grpc.DialOption

	// privileged marks if the agent has a stream authorized clusterID available in
	// the current context.
	privileged bool
}

type ForwarderOption func(*forwarderOptions)

func (o *forwarderOptions) apply(opts ...ForwarderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithAddress(address string) ForwarderOption {
	return func(o *forwarderOptions) {
		o.collectorAddressOverride = address
	}
}

func WithClientConn(cc grpc.ClientConnInterface) ForwarderOption {
	return func(o *forwarderOptions) {
		o.cc = cc
	}
}

func WithLogger(lg *zap.SugaredLogger) ForwarderOption {
	return func(o *forwarderOptions) {
		o.lg = lg
	}
}

func WithDialOptions(opts ...grpc.DialOption) ForwarderOption {
	return func(o *forwarderOptions) {
		o.dialOptions = opts
	}
}

func WithPrivileged(privileged bool) ForwarderOption {
	return func(o *forwarderOptions) {
		o.privileged = privileged
	}
}

func (f *Forwarder) BackgroundInitClient() {
	f.LogsForwarder.Client.BackgroundInitClient(f.initializeLogsForwarder)
	f.TraceForwarder.Client.BackgroundInitClient(f.InitializeTraceForwarder)
}

func (f *Forwarder) ConfigureRoutes(router *gin.Engine) {
	router.POST("/api/agent/otel/v1/traces", f.handleTracePost)
	router.POST("/api/agent/otel/v1/logs", f.handleLogsPost)
	pprof.Register(router, "/debug/plugin_logging/pprof")
}
