package alerting_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Alerting Conditions integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	// ctx := context.Background()
	// var alertingClient alertingv1alpha.AlertingClient
	// // test environment references
	// var env *test.Environment
	BeforeAll(func() {
		var err error
		Expect(err).To(BeNil())
	})
})
