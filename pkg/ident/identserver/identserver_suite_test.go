package identserver_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIdentserver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Identserver Suite")
}
