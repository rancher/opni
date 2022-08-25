package condition_test

import (
	"encoding/json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/template"
	"github.com/rancher/opni/pkg/alerting/condition"
)

var _ = Describe("Alerting Condition Suite", func() {
	When("We parse cortex webhook payloads", func() {
		It("should parse valid payloads to appropriate opni alerting payloads", func() {
			someId := uuid.New().String()
			somename := "some-alert-name"
			exampleWorkingPayload := &condition.MockCortexPayload{
				Message: webhook.Message{
					Data: &template.Data{
						Alerts: []template.Alert{
							{
								Annotations: template.KV{
									"conditionId": someId,
									"alertname":   somename,
								},
							},
						},
					},
				},
			}
			mockBody, err := json.Marshal(&exampleWorkingPayload)
			Expect(err).To(Succeed())
			annotations, err := condition.ParseCortexPayloadBytes(mockBody)
			Expect(err).To(Succeed())
			Expect(annotations).To(HaveLen(1))

			opniResponses, errors := condition.ParseCortexWebhookPayload(annotations)
			Expect(errors).To(HaveLen(1))
			Expect(len(opniResponses)).To(Equal(len(errors)))
			for _, e := range errors {
				Expect(e).To(Succeed())
			}
			Expect(opniResponses[0].ConditionId.GetId()).To(Equal(someId))
			Expect(opniResponses[0].Annotations["alertname"]).To(Equal(somename))
		})

		It("Should errror on invalid cortex webhook payloads", func() {
			somename := "some-alert-name"
			exampleInvalidPayload := &condition.MockCortexPayload{
				Message: webhook.Message{
					Data: &template.Data{
						Alerts: []template.Alert{
							{
								Annotations: template.KV{

									"alertname": somename,
								},
							},
						},
					},
				},
			}
			mockBody, err := json.Marshal(&exampleInvalidPayload)
			Expect(err).To(Succeed())
			annotations, err := condition.ParseCortexPayloadBytes(mockBody)
			Expect(err).To(Succeed())
			opniRequests, errors := condition.ParseCortexWebhookPayload(annotations)
			Expect(errors).To(HaveLen(1))
			Expect(len(opniRequests)).To(Equal(len(errors)))
			Expect(errors[0]).To(HaveOccurred())
			Expect(opniRequests[0]).To(BeNil())
		})

	})
})
