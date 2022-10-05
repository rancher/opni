package k8sutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestK8sutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sutil Suite")
}
