package gpuadapter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGpuadapter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GpuAdapter Suite")
}
