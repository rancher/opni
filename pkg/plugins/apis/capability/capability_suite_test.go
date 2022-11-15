package capability_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCapability(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Capability Suite")
}
