package v1_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func HasDiffHash(condA, condB *alertingv1.AlertCondition) error {
	hashA, err := condA.Hash()
	if err != nil {
		return err
	}
	hashB, err := condB.Hash()
	if err != nil {
		return err
	}
	if hashB == hashA {
		return fmt.Errorf("expected hashA and hashB to be diff : %s vs %s", hashA, hashB)
	}
	return nil
}

func HasSameHash(condA, condB *alertingv1.AlertCondition) error {
	hashA, err := condA.Hash()
	if err != nil {
		return err
	}
	hashB, err := condB.Hash()
	if err != nil {
		return err
	}
	if hashB != hashA {
		return fmt.Errorf("expected hashA and hashB to be same : %s vs %s", hashA, hashB)
	}
	return nil
}

var _ = Describe("Extensions test", Label("unit"), func() {
	When("we calculate hash for alert conditions", func() {
		It("should calculate different hashes when general information is change", func() {
			// name, description, labels, severity, golden signal, attached endpoints
			cond := &alertingv1.AlertCondition{
				Name:              "hello",
				Description:       "",
				Labels:            []string{},
				Severity:          0,
				AlertType:         &alertingv1.AlertTypeDetails{},
				AttachedEndpoints: &alertingv1.AttachedEndpoints{},
				Silence:           &alertingv1.SilenceInfo{},
				LastUpdated:       &timestamppb.Timestamp{},
				Id:                "",
				GoldenSignal:      0,
				OverrideType:      "",
				Metadata:          map[string]string{},
				GroupId:           "",
			}
			condName := util.ProtoClone(cond)
			condName.Name = "hello2"
			condDesc := util.ProtoClone(cond)
			condDesc.Description = "hello2"
			condSev := util.ProtoClone(cond)
			condSev.Severity = 2
			condLastUpdated := util.ProtoClone(cond)
			condLastUpdated.LastUpdated = timestamppb.Now()
			condGoldenSignal := util.ProtoClone(cond)
			condGoldenSignal.GoldenSignal = 1
			condGroup := util.ProtoClone(cond)
			condGroup.GroupId = "hello2"
			diffs := []*alertingv1.AlertCondition{condName, condDesc, condSev, condLastUpdated, condGoldenSignal, condGroup}
			for _, diff := range diffs {
				Expect(HasDiffHash(cond, diff)).To(Succeed())
			}
		})
		Specify("the hash should be different when disconnect spec changes are made", func() {
			disconnect := &alertingv1.AlertCondition{
				AlertType: &alertingv1.AlertTypeDetails{
					Type: &alertingv1.AlertTypeDetails_System{
						System: &alertingv1.AlertConditionSystem{
							ClusterId: &corev1.Reference{
								Id: "agent",
							},
							Timeout: durationpb.New(time.Minute * 10),
						},
					},
				},
			}

			disconnectQuick := util.ProtoClone(disconnect)
			disconnectQuick.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{
							Id: "agent",
						},
						Timeout: durationpb.New(time.Minute * 1),
					},
				},
			}
			disconnectCluster := util.ProtoClone(disconnect)
			disconnectCluster.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{
							Id: "agent2",
						},
						Timeout: durationpb.New(time.Minute * 10),
					},
				},
			}
			diffs := []*alertingv1.AlertCondition{
				disconnectQuick, disconnectCluster,
			}
			for _, diff := range diffs {
				Expect(HasDiffHash(disconnect, diff)).To(Succeed())
			}
		})

		Specify("the hash should be different when capability changes are made", func() {
			capability := &alertingv1.AlertCondition{
				AlertType: &alertingv1.AlertTypeDetails{
					Type: &alertingv1.AlertTypeDetails_DownstreamCapability{
						DownstreamCapability: &alertingv1.AlertConditionDownstreamCapability{
							ClusterId: &corev1.Reference{
								Id: "agent",
							},
							CapabilityState: []string{"foo"},
							For:             durationpb.New(time.Minute * 10),
						},
					},
				},
			}

			capabilityQuick := util.ProtoClone(capability)
			capabilityQuick.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_DownstreamCapability{
					DownstreamCapability: &alertingv1.AlertConditionDownstreamCapability{
						ClusterId: &corev1.Reference{
							Id: "agent",
						},
						CapabilityState: []string{"foo"},
						For:             durationpb.New(time.Minute * 1),
					},
				},
			}
			capabilityCluster := util.ProtoClone(capability)
			capabilityCluster.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_DownstreamCapability{
					DownstreamCapability: &alertingv1.AlertConditionDownstreamCapability{
						ClusterId: &corev1.Reference{
							Id: "agent2",
						},
						CapabilityState: []string{"foo"},
						For:             durationpb.New(time.Minute * 10),
					},
				},
			}
			capabilityState := util.ProtoClone(capability)
			capabilityState.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_DownstreamCapability{
					DownstreamCapability: &alertingv1.AlertConditionDownstreamCapability{
						ClusterId: &corev1.Reference{
							Id: "agent",
						},
						CapabilityState: []string{"foo", "bar"},
						For:             durationpb.New(time.Minute * 10),
					},
				},
			}
			diffs := []*alertingv1.AlertCondition{
				capabilityQuick,
				capabilityState,
				capabilityCluster,
			}
			for _, diff := range diffs {
				Expect(HasDiffHash(capability, diff)).To(Succeed())
			}
		})

		Specify("the hash should be different when prometheus changes are made", func() {
			prometheus := &alertingv1.AlertCondition{
				AlertType: &alertingv1.AlertTypeDetails{
					Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
						PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
							ClusterId: &corev1.Reference{
								Id: "agent",
							},
							Query: "up > 0",
							For:   durationpb.New(time.Minute * 10),
						},
					},
				},
			}

			prometheusCluster := util.ProtoClone(prometheus)
			prometheusCluster.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
					PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
						ClusterId: &corev1.Reference{
							Id: "agent2",
						},
						Query: "up > 0",
						For:   durationpb.New(time.Minute * 10),
					},
				},
			}
			prometheusQuery := util.ProtoClone(prometheus)
			prometheusQuery.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
					PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
						ClusterId: &corev1.Reference{
							Id: "agent",
						},
						Query: "sum(up > 0) > 0",
						For:   durationpb.New(time.Minute * 10),
					},
				},
			}
			prometheusFor := util.ProtoClone(prometheus)
			prometheusFor.AlertType = &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
					PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
						ClusterId: &corev1.Reference{
							Id: "agent",
						},
						Query: "up > 0",
						For:   durationpb.New(time.Minute * 1),
					},
				},
			}
			diffs := []*alertingv1.AlertCondition{
				prometheusCluster,
				prometheusQuery,
				prometheusFor,
			}
			for _, diff := range diffs {
				Expect(HasDiffHash(prometheus, diff)).To(Succeed())
			}
		})

		Specify("the hash should ignore internal metadata changes", func() {
			cond := &alertingv1.AlertCondition{
				Name:        "alarm",
				Description: "alarm description",
				Labels:      []string{"_internal"},
				Severity:    0,
				AttachedEndpoints: &alertingv1.AttachedEndpoints{
					Items:              []*alertingv1.AttachedEndpoint{},
					InitialDelay:       durationpb.New(time.Minute),
					RepeatInterval:     &durationpb.Duration{},
					ThrottlingDuration: &durationpb.Duration{},
					Details:            &alertingv1.EndpointImplementation{},
				},
				Silence:      &alertingv1.SilenceInfo{},
				LastUpdated:  timestamppb.Now(),
				Id:           uuid.New().String(),
				GoldenSignal: 0,
				OverrideType: "",
				Metadata:     map[string]string{},
				GroupId:      "",
			}
			metadataCond := util.ProtoClone(cond)
			metadataCond.Metadata = map[string]string{
				"foo":        "bar",
				"disconnect": "true",
			}

			samesies := []*alertingv1.AlertCondition{
				metadataCond,
			}

			for _, same := range samesies {
				Expect(HasSameHash(cond, same)).To(Succeed())
			}
		})
	})

	When("we redact endpoint secrets", func() {
		It("should redact slack secrets", func() {
			s := &alertingv1.AlertEndpoint{
				Endpoint: &alertingv1.AlertEndpoint_Slack{
					Slack: &alertingv1.SlackEndpoint{
						WebhookUrl: "https://foo.bar",
					},
				},
			}
			s.RedactSecrets()
			Expect(s.GetSlack().WebhookUrl).To(Equal(storagev1.Redacted))
		})

		It("should redact email secrets", func() {
			e := &alertingv1.AlertEndpoint{
				Endpoint: &alertingv1.AlertEndpoint_Email{
					Email: &alertingv1.EmailEndpoint{
						SmtpAuthPassword: lo.ToPtr("password"),
					},
				},
			}
			e.RedactSecrets()
			Expect(*e.GetEmail().SmtpAuthPassword).To(Equal(storagev1.Redacted))
		})

		It("should redact pager duty secrets", func() {
			pg := &alertingv1.AlertEndpoint{
				Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
					PagerDuty: &alertingv1.PagerDutyEndpoint{
						IntegrationKey: "integrationKey",
					},
				},
			}
			pg.RedactSecrets()
			Expect(pg.GetPagerDuty().IntegrationKey).To(Equal(storagev1.Redacted))
		})

		It("should redact webhook secrets", func() {
			e1 := &alertingv1.AlertEndpoint{
				Endpoint: &alertingv1.AlertEndpoint_Webhook{
					Webhook: &alertingv1.WebhookEndpoint{
						Url: "https://foo.bar",
					},
				},
			}
			e1.RedactSecrets()
			Expect(e1.GetWebhook().Url).To(Equal(storagev1.Redacted))

			e2 := &alertingv1.AlertEndpoint{
				Endpoint: &alertingv1.AlertEndpoint_Webhook{
					Webhook: &alertingv1.WebhookEndpoint{
						Url: "https://foo.bar",
						HttpConfig: &alertingv1.HTTPConfig{
							BasicAuth: &alertingv1.BasicAuth{
								Password: "password",
							},
							Authorization: &alertingv1.Authorization{
								Credentials: "credentials",
							},
							Oauth2: &alertingv1.OAuth2{
								ClientSecret: "clientSecret",
							},
						},
					},
				},
			}
			e2.RedactSecrets()
			Expect(e2.GetWebhook().Url).To(Equal(storagev1.Redacted))
			Expect(e2.GetWebhook().HttpConfig.BasicAuth.Password).To(Equal(storagev1.Redacted))
			Expect(e2.GetWebhook().HttpConfig.Authorization.Credentials).To(Equal(storagev1.Redacted))
			Expect(e2.GetWebhook().HttpConfig.Oauth2.ClientSecret).To(Equal(storagev1.Redacted))
		})
	})
})
