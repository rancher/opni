package extensions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/kralicky/yaml/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/extensions"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

func BuildEmbeddedServerNotificationTests(
	routerConstructor func(int) routing.OpniRouting,
	dataset *test.RoutableDataset,
) bool {
	var webPort int
	var opniPort int
	sendMsg := func(client *http.Client, msg config.WebhookMessage, opniPort int) {
		content, err := json.Marshal(msg)
		Expect(err).NotTo(HaveOccurred())
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d%s", opniPort, shared.AlertingDefaultHookName), bytes.NewReader(content))
		Expect(err).NotTo(HaveOccurred())
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	}
	sendMsgAlertManager := func(ctx context.Context, labels, annotations map[string]string, alertManagerPort int) {
		apiNode := backend.NewAlertManagerPostAlertClient(
			ctx,
			fmt.Sprintf("http://localhost:%d", webPort),
			backend.WithExpectClosure(backend.NewExpectStatusOk()),
			backend.WithPostAlertBody(labels[alertingv1.NotificationPropertyOpniUuid], labels, annotations),
		)
		err := apiNode.DoRequest()
		Expect(err).NotTo(HaveOccurred())
	}

	listMsg := func(client *http.Client, listReq *alertingv1.ListMessageRequest, opniPort int) *alertingv1.ListMessageResponse {
		content, err := json.Marshal(listReq)
		Expect(err).NotTo(HaveOccurred())
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d%s", opniPort, "/list"), bytes.NewReader(content))
		Expect(err).NotTo(HaveOccurred())
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var listResp *alertingv1.ListMessageResponse
		err = json.NewDecoder(resp.Body).Decode(&listResp)
		Expect(err).NotTo(HaveOccurred())
		return listResp
	}
	return Describe("EmbeddedServer test suite", Ordered, Label("unit"), func() {
		var client *http.Client
		BeforeAll(func() {
			// start embedded alert manager with config that points to opni embedded server
			Expect(env).NotTo(BeNil())

			freeport, err := freeport.GetFreePort()
			Expect(err).NotTo(HaveOccurred())
			Expect(freeport).NotTo(BeZero())
			opniPort = freeport
			extensions.StartOpniEmbeddedServer(env.Context(), fmt.Sprintf(":%d", opniPort))

			router := routerConstructor(opniPort)
			Expect(tmpConfigDir).NotTo(BeEmpty())
			confFile := path.Join(tmpConfigDir, "alertmanager.yml")
			Expect(confFile).NotTo(BeEmpty())

			config, err := router.BuildConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(config).NotTo(BeNil())
			err = os.WriteFile(confFile, util.Must(yaml.Marshal(config)), 0644)
			Expect(err).NotTo(HaveOccurred())

			port, caF := env.StartEmbeddedAlertManager(env.Context(), confFile, nil)
			DeferCleanup(
				caF,
			)
			webPort = port
			client = http.DefaultClient
		})

		When("we use the embedded opni embedded server", func() {
			It("should handle webhook messages indexed by Opni", func() {
				Expect(webPort).NotTo(BeZero())
				Expect(opniPort).NotTo(BeZero())
				msg := config.WebhookMessage{
					Alerts: config.Alerts{
						{
							Status: "firing",
							Labels: map[string]string{
								alertingv1.NotificationPropertyOpniUuid: uuid.New().String(),
								alertingv1.NotificationPropertySeverity: alertingv1.OpniSeverity_Info.String(),
							},
							Annotations: map[string]string{},
						},
					},
					Version:         "4",
					Receiver:        uuid.New().String(),
					TruncatedAlerts: 0,
					Status:          "firing",
					GroupKey:        uuid.New().String(),
					ExternalURL:     fmt.Sprintf("http://localhost:%d", webPort),
				}
				sendMsg(client, msg, opniPort)
			})

			It("should list webhook messages indexed by Opni", func() {
				Expect(webPort).NotTo(BeZero())
				Expect(opniPort).NotTo(BeZero())

				listReq := &alertingv1.ListMessageRequest{}
				respList := listMsg(client, listReq, opniPort)
				Expect(respList.Items).NotTo(BeNil())
				Expect(respList.Items).To(HaveLen(1))
			})

			Specify("it should dedupe frequency-based persistenced based on group keys and id keys based on what is available", func() {
				listRequest := &alertingv1.ListMessageRequest{
					SeverityFilters: []alertingv1.OpniSeverity{
						alertingv1.OpniSeverity_Warning,
					},
				}
				groupKey := uuid.New().String()
				msgId := uuid.New().String()
				msg := config.WebhookMessage{
					Alerts: config.Alerts{
						{
							Status: "firing",
							Labels: map[string]string{
								alertingv1.NotificationPropertyOpniUuid:  msgId,
								alertingv1.NotificationPropertySeverity:  alertingv1.OpniSeverity_Warning.String(),
								alertingv1.NotificationPropertyDedupeKey: groupKey,
							},
							Annotations: map[string]string{},
						},
					},
					Version:         "4",
					Receiver:        uuid.New().String(),
					TruncatedAlerts: 0,
					Status:          "firing",
					GroupKey:        groupKey,
					ExternalURL:     fmt.Sprintf("http://localhost:%d", webPort),
				}
				sendMsg(client, msg, opniPort)
				respList := listMsg(client, listRequest, opniPort)
				Expect(respList.Items).NotTo(BeNil())
				Expect(respList.Items).To(HaveLen(1))

				// send the same message again with group key but different uuid
				msg.Alerts[0].Labels[alertingv1.NotificationPropertyOpniUuid] = uuid.New().String()
				sendMsg(client, msg, opniPort)
				respList = listMsg(client, listRequest, opniPort)
				Expect(respList.Items).NotTo(BeNil())
				Expect(respList.Items).To(HaveLen(1))

				// send the same message again with uuid but different group key but same uuid
				msg.Alerts[0].Labels[alertingv1.NotificationPropertyOpniUuid] = msgId
				msg.Alerts[0].Labels[alertingv1.NotificationPropertyDedupeKey] = uuid.New().String()

				sendMsg(client, msg, opniPort)
				respList = listMsg(client, listRequest, opniPort)
				Expect(respList.Items).NotTo(BeNil())
				Expect(respList.Items).To(HaveLen(2))

				msg.Alerts[0].Labels[alertingv1.NotificationPropertyOpniUuid] = uuid.New().String()
				msg.Alerts[0].Labels[alertingv1.NotificationPropertyDedupeKey] = uuid.New().String()
				sendMsg(client, msg, opniPort)
				respList = listMsg(client, listRequest, opniPort)
				Expect(respList.Items).NotTo(BeNil())
				Expect(respList.Items).To(HaveLen(3))

				sendMsg(client, msg, opniPort)
				respList = listMsg(client, listRequest, opniPort)
				Expect(respList.Items).NotTo(BeNil())
				Expect(respList.Items).To(HaveLen(3))

			})
		})

		When("we integrate with external AlertManager(s)", func() {
			It("should reset the embedded server state", func() {
				By("verifying there is an input dataset defined")
				Expect(len(dataset.Routables)).NotTo(BeZero())
				Expect(len(dataset.ExpectedPairs)).NotTo(BeZero())

				By("restarting the embedded server")
				freeport, err := freeport.GetFreePort()
				Expect(err).NotTo(HaveOccurred())
				Expect(freeport).NotTo(BeZero())
				opniPort = freeport
				extensions.StartOpniEmbeddedServer(env.Context(), fmt.Sprintf(":%d", opniPort))

				router := routerConstructor(opniPort)
				By("building the required routes for the routables")
				for _, r := range dataset.Routables {
					if r.Namespace() == routing.DefaultSubTreeLabel() {
						// no need to build this one
						continue
					}
					err = router.SetNamespaceSpec(
						"",
						r.GetRoutingLabels()[alertingv1.NotificationPropertyOpniUuid],
						&alertingv1.FullAttachedEndpoints{
							Items: []*alertingv1.FullAttachedEndpoint{},
						},
					)
					Expect(err).NotTo(HaveOccurred())
				}

				Expect(tmpConfigDir).NotTo(BeEmpty())
				confFile := path.Join(tmpConfigDir, "alertmanager.yml")
				Expect(confFile).NotTo(BeEmpty())

				config, err := router.BuildConfig()
				Expect(err).NotTo(HaveOccurred())
				Expect(config).NotTo(BeNil())
				err = os.WriteFile(confFile, util.Must(yaml.Marshal(config)), 0644)
				Expect(err).NotTo(HaveOccurred())

				port, caF := env.StartEmbeddedAlertManager(env.Context(), confFile, nil)
				DeferCleanup(
					caF,
				)
				webPort = port
			})
			It("should persist the routables", func() {

				for _, r := range dataset.Routables {
					sendMsgAlertManager(env.Context(), r.GetRoutingLabels(), r.GetRoutingAnnotations(), webPort)
				}
				Eventually(func() bool {
					_ = webPort
					_ = opniPort
					_ = tmpConfigDir
					for _, pair := range dataset.ExpectedPairs {
						listResp := listMsg(client, pair.A, opniPort)
						if len(listResp.Items) != pair.B {
							return false
						}
					}
					return true
				}, time.Minute*3, time.Second*5,
				).Should(BeTrue())
			})
		})
	})
}

var _ = BuildEmbeddedServerNotificationTests(func(dynamicPort int) routing.OpniRouting {
	return routing.NewDefaultOpniRoutingWithOverrideHook(fmt.Sprintf(
		"http://localhost:%d%s",
		dynamicPort,
		shared.AlertingDefaultHookName,
	))
}, test.NewRoutableDataset())
