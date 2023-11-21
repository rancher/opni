package services_test

import (
	"context"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	amcfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/rancher/opni/pkg/alerting/services"
	alertingv2 "github.com/rancher/opni/pkg/apis/alerting/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage/inmemory"
)

var _ = Describe("Routing service", Label("unit"), Ordered, func() {
	var r services.RoutingStorageService
	BeforeAll(func() {
		r = services.NewRoutingService(
			inmemory.NewKeyValueStore[*amcfg.Config](
				func(in *amcfg.Config) *amcfg.Config { return in },
			),
		)
	})
	When("We use the routing server", func() {
		It("should match input labels to routes", func() {
			err := r.Put(context.TODO(), "agent1", &amcfg.Config{
				Route: &amcfg.Route{
					Receiver: "receiver1",
					Matchers: []*labels.Matcher{
						{
							Name:  "foo",
							Value: "bar",
							Type:  labels.MatchEqual,
						},
					},
				},
				Receivers: []amcfg.Receiver{
					{
						Name: "receiver1",
						SlackConfigs: []*amcfg.SlackConfig{
							{
								APIURL: &amcfg.SecretURL{
									URL: &url.URL{
										Scheme: "https",
										Host:   "slack.com",
									},
								},
							},
						},
					},
				},
			})
			Expect(err).To(Succeed())

			resp, err := r.Matches(context.TODO(), &alertingv2.AlertPayload{
				Clusters: []*corev1.Reference{
					{Id: "agent1"},
				},
				Labels: &alertingv2.Labels{
					Labels: []*alertingv2.Label{
						{
							Name:  "foo",
							Value: "bar",
						},
					},
				},
			})
			Expect(err).To(Succeed())
			Expect(resp).ToNot(BeNil())
			Expect(resp.Receivers).To(HaveLen(1))
			Expect(resp.Receivers).To(HaveKey("agent1"))
			Expect(resp.Receivers["agent1"].Names).To(ConsistOf("receiver1"))
		})
	})
})
