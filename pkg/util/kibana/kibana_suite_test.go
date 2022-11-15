package kibana_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKibana(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kibana Suite")
}
