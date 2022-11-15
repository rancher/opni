package waitctx_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWaitctx(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Waitctx Suite")
}
