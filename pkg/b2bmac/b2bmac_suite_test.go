package b2bmac_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestB2bmac(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Blake2b MAC Suite")
}
