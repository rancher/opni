package b2mac_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestB2mac(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Blake2b MAC Suite")
}
