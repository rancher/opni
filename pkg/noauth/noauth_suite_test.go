package noauth_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
)

func TestNoauth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Noauth Suite")
}
