package urn_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "URN Suite")
}
