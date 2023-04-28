package routing_test

import (
	"fmt"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/test/alerting"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/test/testruntime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

var sharedEndpointSet map[string]*alertingv1.FullAttachedEndpoint

func init() {
	testruntime.IfIntegration(func() {
		sharedEndpointSet = make(map[string]*alertingv1.FullAttachedEndpoint)
		sharedEndpointSet = alerting.CreateRandomSetOfEndpoints()
		var _ = BuildRoutingTreeSuiteTest(
			routing.NewDefaultOpniRouting(),
			alerting.CreateRandomNamespacedTestCases(45, sharedEndpointSet),
			alerting.CreateRandomDefaultNamespacedTestcases(sharedEndpointSet),
			alerting.CreateRandomIndividualEndpointTestcases(sharedEndpointSet),
		)
	})
}

var _ = Describe("Alerting Router defaults", Ordered, Serial, Label("integration"), func() {

	BeforeAll(func() {
		Expect(sharedEndpointSet).ToNot(BeNil())
	})

	When("creating the default routing tree", func() {
		Specify("The default opni routing tree root should be valid for alertmanager", func() {
			fp := freeport.GetFreePort()
			cfg := routing.NewRootNode(fmt.Sprintf("http://localhost:%d", fp))
			Expect(cfg).ToNot(BeNil())
			alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, "routingTreeRoot.yaml", cfg, fp)
		})

		Specify("the opni subtree should be in a valid alertmanager format", func() {
			fp := freeport.GetFreePort()
			cfg := routing.NewRootNode(fmt.Sprintf("http://localhost:%d", fp))
			subtree, recvs := routing.NewOpniSubRoutingTree()
			cfg.Route.Routes = append(cfg.Route.Routes, subtree)
			cfg.Receivers = append(cfg.Receivers, recvs...)
			alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, "routingSubtree.yaml", cfg, fp)
		})

		Specify("the default routing tree of opni routing should be in a valid alertmanager format", func() {
			fp := freeport.GetFreePort()
			cfg := routing.NewRoutingTree(fmt.Sprintf("http://localhost:%d", fp))
			alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, "routingTree.yaml", cfg, fp)
		})
	})
})

func BuildRoutingTreeSuiteTest(
	router routing.OpniRouting,
	conditionSubtreeTestcases []alerting.NamespaceSubTreeTestcase,
	broadcastSubtreeTestcases []alerting.DefaultNamespaceSubTreeTestcase,
	individualEndpointTestcases []alerting.IndividualEndpointTestcase,
) bool {
	return Describe("Alerting Routing tree building tests", Ordered, Serial, Label("integration", "slow"), func() {
		var currentCfg *config.Config
		var step string
		When("manipulating the opni condition routing tree", func() {
			AfterEach(func() {
				By("expecting that the formed alertmanager config is correct")
				fp := freeport.GetFreePort()
				alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, step+".yaml", currentCfg, fp)
			})

			It("should be able to set configurations for routing to endpoints (s)", func() {
				step = "add"
				for _, tc := range conditionSubtreeTestcases {
					strRepr, _ := protojson.Marshal(tc.Endpoints)
					if tc.Op == alerting.OpCreate {
						if len(tc.Endpoints.GetItems()) == 0 {
							Fail("no endpoints to set to a condition")
						}
						err := router.SetNamespaceSpec(tc.Namespace, tc.ConditionId, tc.Endpoints)
						if tc.Code == nil {
							Expect(err).To(Succeed(), fmt.Sprintf("failed to add receiver for config : %s", strRepr))
						} else {
							st, ok := status.FromError(err)
							Expect(ok).To(BeTrue())
							Expect(st.Code()).To(Equal(tc.Code), fmt.Sprintf("failed to expect error code for config : %s", strRepr))
						}
					}
					calculatedConfig, err := router.BuildConfig()
					Expect(err).To(Succeed())
					currentCfg = calculatedConfig
				}
			})

			It("should be able to update configurations for routing to endpoints", func() {
				step = "update"
				for _, tc := range conditionSubtreeTestcases {
					if tc.Op == alerting.OpUpdate {
						if len(tc.Endpoints.GetItems()) == 0 {
							Fail("no endpoints to set to a condition")
						}
						err := router.SetNamespaceSpec(tc.Namespace, tc.ConditionId, tc.Endpoints)
						if tc.Code == nil {
							Expect(err).To(Succeed())
						} else {
							st, ok := status.FromError(err)
							Expect(ok).To(BeTrue())
							Expect(st.Code()).To(Equal(tc.Code))
						}
					}
				}
				calculatedConfig, err := router.BuildConfig()
				Expect(err).To(Succeed())
				currentCfg = calculatedConfig
			})

			It("should be able to delete configurations for routing to endpoints", func() {
				step = "delete"
				for _, tc := range conditionSubtreeTestcases {
					if tc.Op == alerting.OpDelete {
						err := router.SetNamespaceSpec(tc.Namespace, tc.ConditionId, &alertingv1.FullAttachedEndpoints{})
						if tc.Code == nil {
							Expect(err).To(Succeed())
						} else {
							st, ok := status.FromError(err)
							Expect(ok).To(BeTrue())
							Expect(st.Code()).To(Equal(tc.Code))
						}
					}
				}
				calculatedConfig, err := router.BuildConfig()
				Expect(err).To(Succeed())
				currentCfg = calculatedConfig
			})
		})

		When("manipulating the opni default namespaced routing tree", func() {
			AfterEach(func() {
				By("expecting that the formed alertmanager config is correct")
				fp := freeport.GetFreePort()
				alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, step+".yaml", currentCfg, fp)
			})

			It("should be able to add endpoints to the default subtree", func() {
				step = "add-to-default"
				for _, tc := range broadcastSubtreeTestcases {
					err := router.SetDefaultNamespaceConfig(
						tc.Endpoints,
					)
					if tc.Code == nil {
						Expect(err).To(Succeed())
					} else {
						st, ok := status.FromError(err)
						Expect(ok).To(BeTrue())
						Expect(st.Code()).To(Equal(tc.Code))
					}
				}
				calculatedConfig, err := router.BuildConfig()
				Expect(err).To(Succeed())
				currentCfg = calculatedConfig
			})
		})

		When("propagating updates for individual endpoints to the rest of opni routing", func() {
			step = "update-individual"
			It("should be able to update individual endpoints in opni routing", func() {
				for _, tc := range individualEndpointTestcases {
					if tc.Op == alerting.OpUpdate || tc.Op == alerting.OpCreate {
						err := router.UpdateEndpoint(tc.EndpointId, tc.UpdateEndpoint)
						if tc.Code != nil {
							Expect(err).To(HaveOccurred())
							st, ok := status.FromError(err)
							Expect(ok).To(BeTrue())
							Expect(st.Code()).To(Equal(tc.Code))
						} else {
							Expect(err).To(Succeed())
						}
					}
				}
				calculatedConfig, err := router.BuildConfig()
				Expect(err).To(Succeed())
				currentCfg = calculatedConfig
			})

			It("should be able to delete individual endpoints in opni routing", func() {
				step = "delete-individual"
				for _, tc := range individualEndpointTestcases {
					if tc.Op == alerting.OpDelete {
						err := router.DeleteEndpoint(tc.EndpointId)
						if tc.Code != nil {
							Expect(err).To(HaveOccurred())
							st, ok := status.FromError(err)
							Expect(ok).To(BeTrue())
							Expect(st.Code()).To(Equal(tc.Code))
						} else {
							Expect(err).To(Succeed())
						}
					}
				}
				calculatedConfig, err := router.BuildConfig()
				Expect(err).To(Succeed())
				currentCfg = calculatedConfig
			})

			Specify("it should recover exact configs after being persisted", func() {
				step = "recover-config"
				alerting.ExpectToRecoverConfig(router, "no-sync")
			})
		})

		When("syncing user's production configs", func() {
			It("should sync the user's production config into opni's routing tree", func() {
				step = "sync-production-config"
				testcaseFilenames := []string{
					"alerting/alertmanager/basic.yaml",
					// "alerting/alertmanager/production.yaml",
				}

				for _, file := range testcaseFilenames {
					file := file
					By("reading production configs from testdata")
					bytes := testdata.TestData(file)
					By(fmt.Sprintf("expecting the sync operation to succeed for %s", file))
					err := router.SyncExternalConfig(bytes)
					Expect(err).To(Succeed())
					By("expecting that any sync operation will create a valid AlertManager tree")
					calculatedConfig, err := router.BuildConfig()
					Expect(err).To(Succeed())
					currentCfg = calculatedConfig
					alerting.ExpectAlertManagerConfigToBeValid(env.Context(), env, tmpConfigDir, step+"-"+path.Base(file), currentCfg, freeport.GetFreePort())
				}
			})

			// these will be additions that will make the UX on synced configs better
			Specify("Walk, merge & search should be unimplemented", func() {
				_, err := router.Merge(nil)
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.Unimplemented))

				res := router.Search(map[string]string{})
				Expect(res).To(HaveLen(0))

				err = router.Walk(map[string]string{}, func(d int, r *config.Route) error {
					return nil
				})
				Expect(err).To(HaveOccurred())
				st, ok = status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.Unimplemented))
			})
			Specify("it should recover exact configs after being persisted", func() {
				alerting.ExpectToRecoverConfig(router)
			})
		})
	})
}
