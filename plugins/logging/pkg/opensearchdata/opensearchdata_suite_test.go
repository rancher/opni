package opensearchdata_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOpensearchdata(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Opensearchdata Suite")
}
