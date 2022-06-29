package plugins_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCortexadmin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cortexadmin Suite")
}
