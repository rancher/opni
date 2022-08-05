package plugins_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type TestMetadataInput struct {
	Tenant     string
	MetricName string
}

func TestCortexadmin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cortexadmin Suite")
}
