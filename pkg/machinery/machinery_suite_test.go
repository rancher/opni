package machinery_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMachinery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Machinery Suite")
}
