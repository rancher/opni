package etcd

import (
	"testing"
	"time"

	"github.com/lestrrat-go/backoff/v2"
)

var mutexLeaseTtlSeconds int
var defaultBackoff *backoff.ExponentialPolicy

func init() {
	if !testing.Testing() {
		mutexLeaseTtlSeconds = 5
		defaultBackoff = backoff.NewExponentialPolicy(
			backoff.WithMaxRetries(20),
			backoff.WithMinInterval(10*time.Millisecond),
			backoff.WithMaxInterval(1*time.Second),
			backoff.WithJitterFactor(0.1),
			backoff.WithMultiplier(1.5),
		)
	} else {
		mutexLeaseTtlSeconds = 60
		defaultBackoff = backoff.NewExponentialPolicy(
			backoff.WithMaxRetries(100),
			backoff.WithMinInterval(5*time.Millisecond),
			backoff.WithMaxInterval(50*time.Millisecond),
			backoff.WithJitterFactor(0.1),
			backoff.WithMultiplier(1.5),
		)
	}
}
