package ecdh

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestECDH(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EC Diffie-Hellman Suite")
}
