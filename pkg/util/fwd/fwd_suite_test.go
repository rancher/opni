package fwd_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFwd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fwd Suite")
}
