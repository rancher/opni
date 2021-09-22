//go:build !e2e
// +build !e2e

package opnicluster_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestOpnicluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Opnicluster Suite")
}
