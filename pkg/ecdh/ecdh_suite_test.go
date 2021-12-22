package ecdh

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestECDH(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ECDH Suite")
}
