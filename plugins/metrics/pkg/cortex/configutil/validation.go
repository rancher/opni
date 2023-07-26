package configutil

import (
	"sync"

	gokitlog "github.com/go-kit/kit/log"

	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
)

type errLogger struct {
	mu   sync.Mutex
	errs []string
}

func (l *errLogger) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, string(b))
	return len(b), nil
}

func CollectValidationErrorLogs(cfg *cortexops.CortexApplicationConfig) []*cortexops.ValidationError {
	errs := []*cortexops.ValidationError{}
	conf, _, err := CortexAPISpecToCortexConfig(cfg)
	if err != nil {
		errs = append(errs, &cortexops.ValidationError{
			Severity: cortexops.ValidationError_Error,
			Message:  err.Error(),
		})
		return errs
	}
	lg := errLogger{}
	errLogger := gokitlog.NewJSONLogger(&lg)
	if err := conf.Validate(errLogger); err != nil {
		// this error is a bug in cortex; it's always expected since the flusher
		// config only has one field, which is a bool with a default value of true
		// that we always set to false
		if err.Error() != `the Flusher configuration in YAML has been specified as an empty YAML node` {
			errs = append(errs, &cortexops.ValidationError{
				Severity: cortexops.ValidationError_Error,
				Message:  err.Error(),
			})
		}
	}
	for _, err := range lg.errs {
		errs = append(errs, &cortexops.ValidationError{
			Severity: cortexops.ValidationError_Error,
			Message:  err,
		})
	}
	return errs
}
