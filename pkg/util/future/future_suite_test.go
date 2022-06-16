package future_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFuture(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Future Suite")
}
