package routing_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/test"
	"gopkg.in/yaml.v3"
)

var testInternalRouting *routing.OpniInternalRouting

const (
	testConditionId1 = "testConditionId1"
	testConditionId2 = "testConditionId2"
	testConditionId3 = "testConditionId3"
	testConditionId4 = "testConditionId4"
	testConditionId5 = "testConditionId5"
	testConditionId6 = "testConditionId6"
	testEndpointId1  = "testEndpointId1"
	testEndpointId2  = "testEndpointId2"
	testEndpointId3  = "testEndpointId3"
	testEndpointId4  = "testEndpointId4"
	testEndpointId5  = "testEndpointId5"
	testEndpointId6  = "testEndpointId6"
)

var _ = Describe("Internal routing tests", Ordered, Label(test.Unit, test.Slow), func() {
	When("We maintain opni's internal routing information", func() {
		It("should set to valid default values", func() {
			testInternalRouting = routing.NewDefaultOpniInternalRouting()
			Expect(testInternalRouting).NotTo(BeNil())
		})

		It("should error on all get calls on the default values", func() {
			_, err := testInternalRouting.GetFromCondition(testConditionId1)
			Expect(err).NotTo(BeNil())
			_, err = testInternalRouting.Get(testConditionId1, testEndpointId1)
			Expect(err).NotTo(BeNil())
		})

		It("should not add invalid new routing nodes", func() {
			err := testInternalRouting.Add(testConditionId1, testEndpointId1, routing.OpniRoutingMetadata{})
			Expect(err).NotTo(Succeed())
			negative := -2
			err = testInternalRouting.Add(testConditionId1, testConditionId2, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &negative,
			})
			Expect(err).NotTo(Succeed())

		})

		It("should add valid new routing nodes", func() {
			zero := 0
			err := testInternalRouting.Add(
				testConditionId1,
				testEndpointId1,
				routing.OpniRoutingMetadata{
					EndpointType: routing.SlackEndpointInternalId,
					Position:     &zero,
				})
			m, err := testInternalRouting.GetFromCondition(testConditionId1)
			Expect(err).To(BeNil())
			Expect(m).To(HaveLen(1))
			Expect(m[testEndpointId1]).NotTo(BeNil())
			Expect(m[testEndpointId1].EndpointType).To(Equal(routing.SlackEndpointInternalId))
			Expect(m[testEndpointId1].Position).NotTo(BeNil())
			Expect(*m[testEndpointId1].Position).To(Equal(zero))
		})

		It("should error on not found endpointId", func() {
			_, err := testInternalRouting.Get(testConditionId1, testEndpointId3)
			Expect(err).NotTo(Succeed())
		})

		It("should error on not found conditionId", func() {
			_, err := testInternalRouting.GetFromCondition(testConditionId3)
			Expect(err).NotTo(Succeed())
		})

		It("should fail to update with invalid input", func() {
			zero := 0
			err := testInternalRouting.UpdateEndpoint(testConditionId1, testEndpointId3, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).NotTo(Succeed())
			err = testInternalRouting.UpdateEndpoint(testConditionId3, testEndpointId1, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).NotTo(Succeed())
			negative := -2
			err = testInternalRouting.UpdateEndpoint(testConditionId1, testEndpointId1, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &negative,
			})
			Expect(err).NotTo(Succeed())
		})

		It("should update endpoints", func() {
			five := 5
			err := testInternalRouting.UpdateEndpoint(testConditionId1, testEndpointId1, routing.OpniRoutingMetadata{
				EndpointType: routing.EmailEndpointInternalId,
				Position:     &five,
			})
			Expect(err).To(Succeed())
			m, err := testInternalRouting.Get(testConditionId1, testEndpointId1)
			Expect(err).To(Succeed())
			Expect(m.Position).NotTo(BeNil())
			Expect(*m.Position).To(Equal(five))
			Expect(m.EndpointType).To(Equal(routing.EmailEndpointInternalId))
		})

		It("should fail to delete invalid endpoints", func() {
			err := testInternalRouting.RemoveEndpoint(testConditionId1, testEndpointId3)
			Expect(err).NotTo(Succeed())
			err = testInternalRouting.RemoveCondition(testConditionId3)
			Expect(err).NotTo(Succeed())
		})

		It("should delete endpoints", func() {
			err := testInternalRouting.RemoveEndpoint(testConditionId1, testEndpointId1)
			Expect(err).To(Succeed())
			_, err = testInternalRouting.Get(testConditionId1, testEndpointId1)
			Expect(err).NotTo(Succeed())

			zero := 0
			err = testInternalRouting.Add(testConditionId1, testEndpointId3, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).To(Succeed())
			err = testInternalRouting.Add(testConditionId1, testEndpointId4, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).To(Succeed())
			err = testInternalRouting.Add(testConditionId1, testEndpointId5, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).To(Succeed())
			err = testInternalRouting.RemoveEndpoint(testConditionId1, testEndpointId3)
			Expect(err).To(Succeed())
			_, err = testInternalRouting.Get(testConditionId1, testEndpointId3)
			Expect(err).NotTo(Succeed())
			err = testInternalRouting.RemoveCondition(testConditionId1)
			Expect(err).To(Succeed())
			_, err = testInternalRouting.GetFromCondition(testConditionId1)
			Expect(err).NotTo(Succeed())
			err = testInternalRouting.Add(testConditionId1, testEndpointId3, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).To(Succeed())
			err = testInternalRouting.Add(testConditionId1, testEndpointId4, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
			Expect(err).To(Succeed())
			err = testInternalRouting.Add(testConditionId1, testEndpointId5, routing.OpniRoutingMetadata{
				EndpointType: routing.SlackEndpointInternalId,
				Position:     &zero,
			})
		})

		Specify("marshalling a routing tree should recover exactly the same internal routing map", func() {
			bytes, err := yaml.Marshal(testInternalRouting)
			Expect(err).To(BeNil())
			Expect(bytes).NotTo(BeNil())
			newRouting := &routing.OpniInternalRouting{}
			err = yaml.Unmarshal(bytes, newRouting)
			Expect(err).To(BeNil())
			Expect(newRouting).To(Equal(testInternalRouting))
		})
	})
})
