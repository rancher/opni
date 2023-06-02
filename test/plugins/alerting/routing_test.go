package alerting_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/test/alerting"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/durationpb"
)

var defaultHook *alerting.MockIntegrationWebhookServer

func init() {
	testruntime.IfIntegration(func() {
		BuildRoutingLogicTest(
			func() routing.OpniRouting {
				defaultHooks := alerting.NewWebhookMemoryServer(env, "webhook")
				defaultHook = defaultHooks
				return routing.NewDefaultOpniRoutingWithOverrideHook(defaultHook.GetWebhook())
			},
		)
	})
}

func BuildRoutingLogicTest(
	routerConstructor func() routing.OpniRouting,
) bool {
	return Describe("Alerting routing logic translation to physical dispatching", Ordered, Label("integration"), func() {
		var alertingClient client.AlertingClient
		var alertingClient2 client.AlertingClient
		var alertingClient3 client.AlertingClient
		When("setting namespace specs on the routing tree", func() {
			step := "initial"
			var router routing.OpniRouting
			BeforeAll(func() {
				Expect(env).NotTo(BeNil())
				router = routerConstructor()
				Expect(router).NotTo(BeNil())
			})
			AfterEach(func() {
				By(fmt.Sprintf("%s step: expecting that the router can build the config", step))
				currentCfg, err := router.BuildConfig()
				Expect(err).To(Succeed())
				By(fmt.Sprintf("%s step: expecting that the formed alertmanager config is correct", step))
				fp := freeport.GetFreePort()

				alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, step+".yaml", currentCfg, fp)
			})

			It("should be able to dynamically update alert routing", func() {
				step = "dynamic-alert-routing"
				tmpConfigDir := env.GenerateNewTempDirectory("webhook")
				err := os.MkdirAll(tmpConfigDir, 0755)
				Expect(err).To(Succeed())
				By("Creating some test webhook servers")

				servers := alerting.CreateWebhookServer(env, 3)
				server1, server2, server3 := servers[0], servers[1], servers[2]

				condId1, condId2, condId3 := uuid.New().String(), uuid.New().String(), uuid.New().String()
				ns := "test"
				By("routing to a subset of the test webhook servers")
				details1 := &alertingv1.EndpointImplementation{
					Title: "test1",
					Body:  "test1",
				}
				details2 := &alertingv1.EndpointImplementation{
					Title: "test2",
					Body:  "test2",
				}
				details3 := &alertingv1.EndpointImplementation{
					Title: "test3",
					Body:  "test3",
				}
				suiteSpec := &testSpecSuite{
					name:          "dynamic-alert-routing",
					defaultServer: defaultHook,
					specs: []*testSpec{
						{
							namespace: ns,
							id:        condId1,
							servers:   []*alerting.MockIntegrationWebhookServer{server1},
							details:   details1,
						},
						{
							namespace: ns,
							id:        condId2,
							servers:   []*alerting.MockIntegrationWebhookServer{server1, server2},
							details:   details2,
						},
						{
							namespace: ns,
							id:        condId3,
							servers:   []*alerting.MockIntegrationWebhookServer{server1, server2, server3},
							details:   details3,
						},
					},
				}
				By("setting the router to the namespace specs")
				for _, spec := range suiteSpec.specs {
					endpoints := lo.Map(
						spec.servers,
						func(server *alerting.MockIntegrationWebhookServer, _ int) *alertingv1.FullAttachedEndpoint {
							return &alertingv1.FullAttachedEndpoint{
								AlertEndpoint: server.Endpoint(),
								EndpointId:    server.Endpoint().Id,
								Details:       spec.details,
							}
						})
					err = router.SetNamespaceSpec(
						spec.namespace,
						spec.id,
						&alertingv1.FullAttachedEndpoints{
							Items:              endpoints,
							Details:            spec.details,
							InitialDelay:       durationpb.New(time.Second * 1),
							ThrottlingDuration: durationpb.New(time.Second * 1),
						},
					)
					Expect(err).To(Succeed())
				}

				By("running alertmanager with this config")
				amPort, ca := alerting.RunAlertManager(env, router, tmpConfigDir, step+".yaml")
				alertingClient = client.NewClient(
					nil,
					fmt.Sprintf("http://localhost:%d", amPort),
					fmt.Sprintf("http://localhost:%d", 0),
				)
				defer ca()
				By("sending alerts to each condition in the router")
				for _, spec := range suiteSpec.specs {
					err := alertingClient.AlertClient().PostAlarm(context.TODO(), client.AlertObject{
						Id: spec.id,
						Labels: map[string]string{
							ns: spec.id,
						},
						Annotations: map[string]string{},
					})
					Expect(err).To(Succeed())
				}
				Eventually(func() error {
					return suiteSpec.ExpectAlertsToBeRouted(amPort)
				}, time.Second*15, time.Second*1).Should(Succeed())
				ca()
				server1.ClearBuffer()
				server2.ClearBuffer()
				server3.ClearBuffer()
				defaultHook.ClearBuffer()

				By("deleting a random server endpoint")
				// ok
				err = router.DeleteEndpoint(suiteSpec.specs[0].servers[0].Endpoint().Id)
				Expect(err).To(Succeed())
				for _, spec := range suiteSpec.specs {
					spec.servers = spec.servers[1:]
				}

				amPort2, ca2 := alerting.RunAlertManager(env, router, tmpConfigDir, step+".yaml")
				alertingClient2 = client.NewClient(
					nil,
					fmt.Sprintf("http://localhost:%d", amPort2),
					fmt.Sprintf("http://localhost:%d", 0),
				)
				defer ca2()
				By("sending alerts to each condition in the router")
				for _, spec := range suiteSpec.specs {
					err := alertingClient2.AlertClient().PostAlarm(context.TODO(), client.AlertObject{
						Id: spec.id,
						Labels: map[string]string{
							ns: spec.id,
						},
						Annotations: map[string]string{},
					})
					Expect(err).To(Succeed())
				}
				Eventually(func() error {
					return suiteSpec.ExpectAlertsToBeRouted(amPort2)
				}, time.Second*15, time.Second*1).Should(Succeed())
				ca2()

				By("updating an endpoint to another endpoint")

				server1.ClearBuffer()
				server2.ClearBuffer()
				server3.ClearBuffer()
				defaultHook.ClearBuffer()

				err = router.UpdateEndpoint(server2.Endpoint().Id, server1.Endpoint())
				Expect(err).To(Succeed())
				for _, spec := range suiteSpec.specs {
					if len(spec.servers) != 0 {
						spec.servers[0] = server1
					}
				}

				By("send an an alert to each specs")
				amPort3, ca3 := alerting.RunAlertManager(env, router, tmpConfigDir, step+".yaml")
				defer ca3()
				alertingClient3 = client.NewClient(
					nil,
					fmt.Sprintf("http://localhost:%d", amPort3),
					fmt.Sprintf("http://localhost:%d", 0),
				)
				By("sending alerts to each condition in the router")
				for _, spec := range suiteSpec.specs {
					err := alertingClient3.AlertClient().PostAlarm(context.TODO(), client.AlertObject{
						Id: spec.id,
						Labels: map[string]string{
							ns: spec.id,
						},
						Annotations: map[string]string{},
					})
					Expect(err).To(Succeed())
				}
				Eventually(func() error {
					return suiteSpec.ExpectAlertsToBeRouted(amPort3)
				}, time.Second*15, time.Second*1).Should(Succeed())
				ca3()
			})
		})
	})
}

type testSpecSuite struct {
	name          string
	specs         []*testSpec
	defaultServer *alerting.MockIntegrationWebhookServer
}

type testSpec struct {
	namespace string
	id        string
	servers   []*alerting.MockIntegrationWebhookServer
	details   *alertingv1.EndpointImplementation
}

// FIXME: this expects that the router interface implementations builds things in the format specified by OpniRouterV1
func (t testSpecSuite) ExpectAlertsToBeRouted(amPort int) error {
	By("getting the AlertManager state")
	alertingClient := client.NewClient(
		nil,
		fmt.Sprintf("http://localhost:%d", amPort),
		fmt.Sprintf("http://localhost:%d", 0),
	)
	ags, err := alertingClient.ListAlerts(context.Background())
	Expect(err).To(Succeed())
	for _, spec := range t.specs {
		ns := spec.namespace
		conditionId := spec.id
		found := false
		for _, ag := range ags {
			for _, alert := range ag.Alerts {
				for labelName, label := range alert.Labels {
					if labelName == ns && label == conditionId {
						found = true
						names := lo.Map(alert.Receivers, func(r *models.Receiver, _ int) string {
							return *r.Name
						})
						// each namespace should contain the default webhook
						if !slices.Contains(names, shared.AlertingHookReceiverName) {
							return fmt.Errorf("expected to find finalizer for '%s'=%s in receivers: %s", ns, conditionId, strings.Join(names, ","))
						}
						val := lo.Count(names, shared.AlertingHookReceiverName)
						if val != 1 {
							return fmt.Errorf("expected to find only one copy of finalizer for '%s'=%s in receivers: %s", ns, conditionId, strings.Join(names, ","))
						}
					}
				}
			}
		}
		Expect(found).To(BeTrue())
	}
	// Addr is unique for each server
	uniqServers := map[string]lo.Tuple2[*alerting.MockIntegrationWebhookServer, string]{}
	// Addr
	expectedIds := map[string][]string{}
	for _, spec := range t.specs {
		for _, server := range spec.servers {
			if _, ok := uniqServers[server.Addr]; !ok {
				uniqServers[server.Addr] = lo.Tuple2[*alerting.MockIntegrationWebhookServer, string]{A: server, B: spec.namespace}
			}
			if _, ok := expectedIds[server.Addr]; !ok {
				expectedIds[server.Addr] = []string{}
			}
			expectedIds[server.Addr] = append(expectedIds[server.Addr], spec.id)
		}
	}

	Expect(expectedIds).NotTo(HaveLen(0))
	for _, server := range uniqServers {
		ids := []string{}
		for _, msg := range server.A.GetBuffer() {
			for _, alert := range msg.Alerts {
				if _, ok := alert.Labels[server.B]; ok {
					// namespace is present
					ids = append(ids, alert.Labels[server.B])
				}
			}
		}
		ids = lo.Uniq(ids)
		slices.SortFunc(ids, func(a, b string) bool {
			return a < b
		})
		slices.SortFunc(expectedIds[server.A.Addr], func(a, b string) bool {
			return a < b
		})

		if !slices.Equal(ids, expectedIds[server.A.Addr]) {
			return fmt.Errorf("expected to find ids %s in server %s, but found %s", strings.Join(expectedIds[server.A.Addr], ","), server.A.Addr, strings.Join(ids, ","))
		}
	}

	// default hook should have persisted messages from each condition
	ids := []string{}
	namespaces := []string{}
	for _, spec := range t.specs {
		ids = append(ids, spec.id)
		namespaces = append(namespaces, spec.namespace)
	}
	ids = lo.Uniq(ids)
	namespaces = lo.Uniq(namespaces)

	foundIds := []string{}
	for _, msg := range t.defaultServer.GetBuffer() {
		for _, alert := range msg.Alerts {
			for _, ns := range namespaces {
				if _, ok := alert.Labels[ns]; ok {
					// namespace is present
					foundIds = append(foundIds, alert.Labels[ns])
				}
			}
		}
	}
	foundIds = lo.Uniq(foundIds)
	slices.SortFunc(ids, func(a, b string) bool {
		return a < b
	})
	slices.SortFunc(foundIds, func(a, b string) bool {
		return a < b
	})

	if !slices.Equal(ids, foundIds) {
		return fmt.Errorf("expected to find ids %s in default server, but found %s", strings.Join(ids, ","), strings.Join(foundIds, ","))
	}

	return nil
}
