package v1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
)

func TestV1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage API Suite")
}
