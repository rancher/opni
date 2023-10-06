package client_test

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"

	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

func opniAlerts(ag alertmanagerv2.AlertGroups) alertmanagerv2.AlertGroups {
	return lo.Filter(ag, func(item *alertmanagerv2.AlertGroup, _ int) bool {
		_, ok := item.Labels["opni_uuid"]
		return ok
	})
}

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
			It("should intially report an empty list of opni alerts", func() {
				ag, err := cl.ListAlerts(env.Context())
				Expect(err).To(Succeed())
				Expect(ag).NotTo(BeNil())

				opniAlerts := opniAlerts(ag)
				Expect(opniAlerts).To(HaveLen(0))
			})

			It("Should be able to post an alarm", func() {
				err := cl.PostAlarm(env.Context(), client.AlertObject{
					Id: "test",
					Labels: map[string]string{
						message.NotificationPropertyFingerprint: strconv.Itoa(int(time.Now().Unix())),
					},
					Annotations: map[string]string{
						message.NotificationContentAlarmName: "test-alarm",
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
					opniAlerts := opniAlerts(ag)
					if len(opniAlerts) != 1 {
						return fmt.Errorf("length of opniAlerts is not 1")
					}
					if len(opniAlerts[0].Alerts) != 1 {
						return fmt.Errorf("opniAlerts[0].Alerts is not of length 1")
					}
					if opniAlerts[0].Alerts[0].Labels["opni_uuid"] != "test" {
						return fmt.Errorf("opni_uuid label is not test")
					}
					return nil
				}).Should(Succeed())

				err = cl.PostAlarm(env.Context(), client.AlertObject{
					Id: "test2",
					Labels: map[string]string{
						message.NotificationPropertyFingerprint: strconv.Itoa(int(time.Now().Unix())),
						"foo":                                   "bar",
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
				Expect(silenceId).NotTo(BeEmpty())

				// check silence list
				Eventually(func() error {
					silences, err := sl.ListSilences(env.Context())
					if err != nil {
						return err
					}
					for _, silence := range silences {
						if len(silence.Matchers) != 1 {
							return fmt.Errorf("silence matchers is not of length 1")
						}
						if *silence.ID != silenceId {
							return fmt.Errorf("silence id is not %s", silenceId)
						}
						if *silence.Matchers[0].Name != "opni_uuid" {
							return fmt.Errorf("silence matcher name is not opni_uuid")
						}
						if *silence.Matchers[0].Value != id {
							return fmt.Errorf("silence matcher value is not %s", id)
						}
						t, err := time.Parse(time.RFC3339Nano, string(silence.EndsAt.String()))
						if err != nil {
							return err
						}
						if t.Before(time.Now()) {
							return fmt.Errorf("silence endsAt is before now")
						}
						if t.Before(time.Now().Add(time.Minute)) {
							return fmt.Errorf("silence endsAt is before 1 minute from now")
						}
						if t.After(time.Now().Add(time.Minute * 6)) {
							return fmt.Errorf("silence endsAt is after 6 minutes from now, should be approximately 5")
						}
					}
					return nil
				}, time.Second*5).Should(Succeed())

				// update
				By("updating the existing silence to be longer")
				_, err = sl.PostSilence(
					env.Context(),
					id,
					time.Hour*5,
					lo.ToPtr(silenceId),
				)
				Expect(err).To(Succeed())

				Eventually(func() error {
					silences, err := sl.ListSilences(env.Context())
					if err != nil {
						return err
					}
					for _, silence := range silences {
						if len(silence.Matchers) != 1 {
							return fmt.Errorf("silence matchers is not of length 1")
						}
						if *silence.ID != silenceId {
							return fmt.Errorf("silence id is not %s", silenceId)
						}
						if *silence.Matchers[0].Name != "opni_uuid" {
							return fmt.Errorf("silence matcher name is not opni_uuid")
						}
						if *silence.Matchers[0].Value != id {
							return fmt.Errorf("silence matcher value is not %s", id)
						}
						t, err := time.Parse(time.RFC3339Nano, string(silence.EndsAt.String()))
						if err != nil {
							return err
						}
						if t.Before(time.Now()) {
							return fmt.Errorf("silence endsAt is before now")
						}
						if t.Before(time.Now().Add(time.Minute)) {
							return fmt.Errorf("silence endsAt is before 1 minute from now")
						}
						if t.Before(time.Now().Add(time.Minute * 10)) {
							return fmt.Errorf("silence endsAt is before 10 minutes from now, should be approximately 5 hours from now")
						}
					}
					return nil
				}, time.Second*5).Should(Succeed())
			})
		})
		When("we use the querier client to retrieve alert and notification data", func() {
			It("should list received alarm messages", func() {
				now := time.Now()
				Eventually(func() error {
					msgs, err := ql.ListAlarmMessages(env.Context(), &alertingv1.ListAlarmMessageRequest{
						ConditionId:  &alertingv1.ConditionReference{Id: "test"},
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

func BuildProxyClientTestSuite(
	name string,
	clBuilder func() client.ProxyClient,
) bool {
	return Describe(fmt.Sprintf("proxy client : %s", name), Ordered, Label("integration"), func() {
		var cl client.ProxyClient
		BeforeAll(func() {
			cl = clBuilder()
		})
		When("Using the Proxy client", func() {
			It("should construct a proxy target URL", func() {
				Expect(func() { cl.ProxyURL() }).NotTo(Panic())
				Expect(cl.ProxyURL()).NotTo(BeNil())
			})
		})
	})
}

func init() {
	BuildStatusClientTestSuite(
		"standalone alertmanager",
		func() client.StatusClient {
			return cl.StatusClient()
		},
	)

	BuilderMemberlistClientTestSuite(
		"standalone alertmanager",
		func() client.MemberlistClient {
			return cl.MemberlistClient()
		},
		1,
	)

	BuildConfigClientTestSuite(
		"standalone alertmanager",
		func() client.ConfigClient {
			return cl.ConfigClient()
		},
	)

	BuildAlertAndQuerierClientTestSuite(
		"standalone alertmanager",
		func() client.AlertClient {
			return cl.AlertClient()
		},
		func() client.SilenceClient {
			return cl.SilenceClient()
		},
		func() client.QueryClient {
			return cl.QueryClient()
		},
	)

	BuildControlClientTestSuite(
		"standalone alertmanager",
		func() client.ControlClient {
			return cl.ControlClient()
		},
	)

	BuildProxyClientTestSuite(
		"standalone alertmanager",
		func() client.ProxyClient {
			return cl.ProxyClient()
		},
	)

	BuildStatusClientTestSuite(
		"ha alertmanager",
		func() client.StatusClient {
			return clHA.StatusClient()
		},
	)

	BuilderMemberlistClientTestSuite(
		"ha alertmanager",
		func() client.MemberlistClient {
			return clHA.MemberlistClient()
		},
		3,
	)

	BuildConfigClientTestSuite(
		"ha alertmanager",
		func() client.ConfigClient {
			return clHA.ConfigClient()
		},
	)

	BuildAlertAndQuerierClientTestSuite(
		"ha alertmanager",
		func() client.AlertClient {
			return clHA.AlertClient()
		},
		func() client.SilenceClient {
			return clHA.SilenceClient()
		},
		func() client.QueryClient {
			return clHA.QueryClient()
		},
	)
	BuildControlClientTestSuite(
		"ha alertmanager",
		func() client.ControlClient {
			return clHA.ControlClient()
		},
	)

	BuildProxyClientTestSuite(
		"ha proxy client",
		func() client.ProxyClient {
			return clHA.ProxyClient()
		},
	)
}
