package configutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfigutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configutil Suite")
}
