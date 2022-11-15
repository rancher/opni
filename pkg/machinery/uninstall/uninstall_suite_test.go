package uninstall_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUninstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Uninstall Suite")
}
