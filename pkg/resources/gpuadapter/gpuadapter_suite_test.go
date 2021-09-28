//go:build !e2e
// +build !e2e

package gpuadapter_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGpuadapter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GpuAdapter Suite")
}
