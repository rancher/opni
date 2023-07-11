package driverutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDriverutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Driverutil Suite")
}
