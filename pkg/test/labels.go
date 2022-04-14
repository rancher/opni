package test

import (
	"os"

	"github.com/onsi/ginkgo/v2"
)

const (
	Unit          = "unit"
	Integration   = "integration"
	E2E           = "e2e"
	Slow          = "slow"
	TimeSensitive = "time-sensitive"
)

func EnableInCI[T any](decorator T) any {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return decorator
	}
	return ginkgo.Labels{}
}
