package keyring_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestKeyring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Keyring Suite")
}
