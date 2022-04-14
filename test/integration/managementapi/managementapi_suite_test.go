package integration_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestManagementapi(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ManagementAPI Suite")
}
