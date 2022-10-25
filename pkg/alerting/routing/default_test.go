package routing_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Full fledged dynamic opni routing tests", Ordered, Label(test.Unit, test.Slow), func() {
	When("we need the test to have the correct imports lol", func() {
		var err error
		Expect(err).To(BeNil())
	})

	When("use opni's default routing trees", func() {
		It("should be able to construct a default routing tree struct", func() {
			_ = routing.NewDefaultRoutingTree("http://localhost:8080")
		})

		It("AlertManager should accept this config", func() {
			r := routing.NewDefaultRoutingTree("http://localhost:8080")
			bytes, err := r.Marshal()
			Expect(err).To(Succeed())
			validateErr := backend.ValidateIncomingConfig(string(bytes), logger.NewPluginLogger().Named("alerting"))
			Expect(validateErr).To(Succeed())
		})
	})
})
