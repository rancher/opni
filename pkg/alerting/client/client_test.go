package client_test

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

func BuildStatusClientTestSuite(
	name string,
	clBuilder func() client.StatusClient,
) bool {
	return Describe(fmt.Sprintf("status client %s", name), Ordered, Label("integration"), func() {
		var cl client.StatusClient
		BeforeAll(func() {
			cl = clBuilder()
		})
		When("Using the status client", func() {
			It("the server should respond with ready", func() {
				err := cl.Ready(env.Context())
				Expect(err).To(Succeed())
			})
			It("should report the status", func() {
				_, err := cl.Status(env.Context())
				Expect(err).To(Succeed())
			})
		})
	})
}

func BuilderMemberlistClientTestSuite(
	name string,
	clBuilder func() client.MemberlistClient,
	numPeers int,
) bool {
	return Describe(fmt.Sprintf("memberlist client %s", name), Ordered, Label("integration"), func() {
		var cl client.MemberlistClient
		BeforeAll(func() {
			cl = clBuilder()
		})

		When("Using the memberlist client", func() {
			It("should report the alerting members", func() {
				Eventually(func() []client.AlertingPeer {
					return cl.MemberPeers()
				}).Should(HaveLen(numPeers))
			})
		})
	})
}

func BuildConfigClientTestSuite(
	name string,
	clBuilder func() client.ConfigClient,
) bool {
	return Describe(fmt.Sprintf("config client %s", name), Ordered, Label("integration"), func() {
		var cl client.ConfigClient
		BeforeAll(func() {
			cl = clBuilder()
		})

		When("Using the config client", func() {
			It("should report a list of the receivers", func() {
				Eventually(func() []alertmanagerv2.Receiver {
					resp, err := cl.ListReceivers(env.Context())
					if err != nil {
						return []alertmanagerv2.Receiver{}
					}
					return resp
					// default receivers for each severity + default hook
				}).Should(HaveLen(len(alertingv1.OpniSeverity_value) + 1))
			})

			It("should report specific receivers", func() {
				Eventually(func() error {
					_, err := cl.GetReceiver(env.Context(), shared.AlertingHookReceiverName)
					return err
				}).Should(BeNil())
			})

			It("should report errors when receivers are not found", func() {
				_, err := cl.GetReceiver(env.Context(), "not-a-receiver")
				Expect(err).NotTo(BeNil())
			})
		})
	})
}

func BuildAlertAndQuerierClientTestSuite(
	name string,
	alertBuilder func() client.AlertClient,
	silenceBuilder func() client.SilenceClient,
	querierBuilder func() client.QueryClient,
) bool {
	return Describe(fmt.Sprintf("alert client %s", name), Ordered, Label("integration"), func() {
		var cl client.AlertClient
		var ql client.QueryClient
		var sl client.SilenceClient
		BeforeAll(func() {
			cl = alertBuilder()
			ql = querierBuilder()
			sl = silenceBuilder()
		})
		When("Using the alert client", func() {
			It("should intially report an empty list of alerts", func() {
				ag, err := cl.ListAlerts(env.Context())
				Expect(err).To(Succeed())
				Expect(ag).NotTo(BeNil())
				Expect(ag).To(HaveLen(0))
			})

			It("Should be able to post an alarm", func() {
				err := cl.PostAlarm(env.Context(), client.AlertObject{
					Id: "test",
					Labels: map[string]string{
						alertingv1.NotificationPropertyFingerprint: strconv.Itoa(int(time.Now().Unix())),
					},
					Annotations: map[string]string{
						shared.OpniAlarmNameAnnotation: "test-alarm",
					},
				})
				Expect(err).To(Succeed())
				Eventually(func() error {
					ag, err := cl.ListAlerts(env.Context())
					if err != nil {
						return err
					}
					if ag == nil {
						return fmt.Errorf("ag is nil")
					}
					if len(ag) != 1 {
						return fmt.Errorf("length of ag is not 1")
					}
					if len(ag[0].Alerts) != 1 {
						return fmt.Errorf("ag[0].Alerts is not of length 1")
					}
					if ag[0].Alerts[0].Labels["opni_uuid"] != "test" {
						return fmt.Errorf("opni_uuid label is not test")
					}
					return nil
				}).Should(Succeed())

				err = cl.PostAlarm(env.Context(), client.AlertObject{
					Id: "test2",
					Labels: map[string]string{
						alertingv1.NotificationPropertyFingerprint: strconv.Itoa(int(time.Now().Unix())),
						"foo": "bar",
					},
					Annotations: map[string]string{},
				})
				Expect(err).To(Succeed())

				Eventually(func() error {
					alert, err := cl.GetAlert(env.Context(), []*labels.Matcher{
						{
							Type:  labels.MatchEqual,
							Name:  "opni_uuid",
							Value: "test2",
						},
					})
					if err != nil {
						return err
					}
					if alert == nil {
						return fmt.Errorf("alert is nil")
					}
					if len(alert) != 1 {
						return fmt.Errorf("length of alert is not 1")
					}
					if alert[0].Labels["foo"] != "bar" {
						return fmt.Errorf("label foo is not bar")
					}
					return nil
				}).Should(Succeed())

				alert, err := cl.GetAlert(env.Context(), []*labels.Matcher{
					{
						Type:  labels.MatchEqual,
						Name:  "opni_uuid",
						Value: "test2",
					},
				})
				Expect(err).To(Succeed())
				Expect(alert).NotTo(BeNil())
				Expect(alert).To(HaveLen(1))
				Expect(alert[0].Alert.Labels["foo"]).To(Equal("bar"))
			})

			It("should be able to post a notification", func() {
				err := cl.PostNotification(env.Context(), client.AlertObject{
					Id:          "on-demand",
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				})
				Expect(err).To(Succeed())

				Eventually(func() error {
					ag, err := cl.GetAlert(env.Context(), []*labels.Matcher{
						{
							Type:  labels.MatchEqual,
							Name:  "opni_uuid",
							Value: "on-demand",
						},
					})
					Expect(err).To(Succeed())
					Expect(ag).NotTo(BeNil())
					Expect(ag).To(HaveLen(1))
					Expect(ag[0].Labels).To(HaveLen(2))
					Expect(ag[0].EndsAt).NotTo(BeNil())
					t, err := time.Parse(time.RFC3339Nano, string(ag[0].EndsAt.String()))
					Expect(err).To(Succeed())
					Expect(t).To(BeTemporally(">", time.Now()))
					Expect(t).To(BeTemporally("<", time.Now().Add(time.Minute*2)))
					return nil
				}).Should(Succeed())
			})
		})

		When("we use the silence client", func() {
			It("should initially list an empty set of silences", func() {
				silences, err := sl.ListSilences(env.Context())
				Expect(err).To(Succeed())
				Expect(silences).To(BeEmpty())
			})

			It("should be able to create & update a silence", func() {
				By("creating an alert object to silence")
				id := uuid.New().String()

				err := cl.PostNotification(env.Context(), client.AlertObject{
					Id:          id,
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				})
				Expect(err).To(Succeed())

				By("create a new silence")
				silenceId, err := sl.PostSilence(
					env.Context(),
					id,
					time.Minute*5,
					nil,
				)
				Expect(err).To(Succeed())

				// check silence list
				silences, err := sl.ListSilences(env.Context())
				Expect(err).To(Succeed())
				for _, silence := range silences {
					Expect(silence.Matchers).To(HaveLen(1))
					Expect(*silence.ID).To(Equal(silenceId))
					Expect(*silence.Matchers[0].Name).To(Equal("opni_uuid"))
					Expect(*silence.Matchers[0].Value).To(Equal(id))
					t, err := time.Parse(time.RFC3339Nano, string(silence.EndsAt.String()))
					Expect(err).To(Succeed())
					Expect(t).To(BeTemporally(">", time.Now()))
					Expect(t).To(BeTemporally(">", time.Now().Add(time.Minute)))
					Expect(t).To(BeTemporally("<", time.Now().Add(6*time.Minute)))
				}

				// update
				By("updating the existing silence to be longer")
				_, err = sl.PostSilence(
					env.Context(),
					id,
					time.Hour*5,
					lo.ToPtr(silenceId),
				)
				Expect(err).To(Succeed())

				// check silence list
				silences, err = sl.ListSilences(env.Context())
				Expect(err).To(Succeed())

				for _, silence := range silences {
					Expect(silence.Matchers).To(HaveLen(1))
					Expect(*silence.ID).To(Equal(silenceId))
					Expect(*silence.Matchers[0].Name).To(Equal("opni_uuid"))
					Expect(*silence.Matchers[0].Value).To(Equal(id))
					t, err := time.Parse(time.RFC3339Nano, string(silence.EndsAt.String()))
					Expect(err).To(Succeed())
					Expect(t).To(BeTemporally(">", time.Now()))
					Expect(t).To(BeTemporally(">", time.Now().Add(time.Minute)))
					Expect(t).To(BeTemporally(">", time.Now().Add(10*time.Minute)))
				}
			})
		})
		When("we use the querier client to retrieve alert and notification data", func() {
			It("should list received alarm messages", func() {
				now := time.Now()
				Eventually(func() error {
					msgs, err := ql.ListAlarmMessages(env.Context(), &alertingv1.ListAlarmMessageRequest{
						ConditionId:  "test",
						Fingerprints: []string{},
						Start:        timestamppb.New(now.Add(-time.Minute)),
						End:          timestamppb.New(now.Add(time.Minute)),
					})
					if err != nil {
						return err
					}
					if len(msgs.Items) == 0 {
						return fmt.Errorf("length of msgs is 0")
					}
					if len(msgs.Items) > 1 {
						return fmt.Errorf("expected no more than one message")
					}
					return nil
				}, time.Second*15, time.Second*5).Should(Succeed())
			})

			It("should list received notification messages", func() {
				Eventually(func() error {
					msgs, err := ql.ListNotificationMessages(env.Context(), &alertingv1.ListNotificationRequest{
						GoldenSignalFilters: []alertingv1.GoldenSignal{
							alertingv1.GoldenSignal_Custom,
						},
						SeverityFilters: []alertingv1.OpniSeverity{
							alertingv1.OpniSeverity_Critical,
							alertingv1.OpniSeverity_Error,
							alertingv1.OpniSeverity_Warning,
							alertingv1.OpniSeverity_Info,
						},
					})
					if err != nil {
						return err
					}
					if len(msgs.Items) == 0 {
						return fmt.Errorf("length of msgs is 0")
					}
					return nil
				}, time.Second*15, time.Second*5).Should(Succeed())
			})
		})

	})
}

func BuildControlClientTestSuite(
	name string,
	clBuilder func() client.ControlClient,
) bool {
	return Describe(fmt.Sprintf("control client %s", name), Ordered, Label("integration"), func() {
		var cl client.ControlClient
		BeforeAll(func() {
			cl = clBuilder()
		})
		When("Using the control client", func() {
			It("should be able to reload the alertmanager instance", func() {
				err := cl.Reload(env.Context())
				Expect(err).To(Succeed())
			})
		})
	})
}

func init() {
	BuildStatusClientTestSuite(
		"standalone alertmanager",
		func() client.StatusClient {
			return cl
		},
	)

	BuilderMemberlistClientTestSuite(
		"standalone alertmanager",
		func() client.MemberlistClient {
			return cl
		},
		1,
	)

	BuildConfigClientTestSuite(
		"standalone alertmanager",
		func() client.ConfigClient {
			return cl
		},
	)

	BuildAlertAndQuerierClientTestSuite(
		"standalone alertmanager",
		func() client.AlertClient {
			return cl
		},
		func() client.SilenceClient {
			return cl
		},
		func() client.QueryClient {
			return cl
		},
	)

	BuildControlClientTestSuite(
		"standalone alertmanager",
		func() client.ControlClient {
			return cl
		},
	)

	BuildStatusClientTestSuite(
		"ha alertmanager",
		func() client.StatusClient {
			return clHA
		},
	)

	BuilderMemberlistClientTestSuite(
		"ha alertmanager",
		func() client.MemberlistClient {
			return clHA
		},
		3,
	)

	BuildConfigClientTestSuite(
		"ha alertmanager",
		func() client.ConfigClient {
			return clHA
		},
	)

	BuildAlertAndQuerierClientTestSuite(
		"ha alertmanager",
		func() client.AlertClient {
			return clHA
		},
		func() client.SilenceClient {
			return clHA
		},
		func() client.QueryClient {
			return clHA
		},
	)
	BuildControlClientTestSuite(
		"ha alertmanager",
		func() client.ControlClient {
			return clHA
		},
	)
}
