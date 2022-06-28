package slo_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSlo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Slo Suite")
}
