package apiextensions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApiextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apiextensions Suite")
}
