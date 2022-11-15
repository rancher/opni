package modeltraining_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestModeltraining(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Modeltraining Suite")
}
