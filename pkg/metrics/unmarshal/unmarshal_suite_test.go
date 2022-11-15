package unmarshal_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUnmarshal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Unmarshal Suite")
}
