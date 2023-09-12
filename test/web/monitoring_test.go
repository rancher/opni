package web_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = XDescribe("Monitoring", Ordered, Label("web"), func() {
	var mgmtClient managementv1.ManagementClient
	var adminClient cortexadmin.CortexAdminClient
	var opsClient cortexops.CortexOpsClient
	BeforeAll(func() {
		Expect(true).To(BeTrue())
		Expect(env.BootstrapNewAgent("monitoring-test-agent1", test.WithLocalAgent())).To(Succeed())
		Expect(env.BootstrapNewAgent("monitoring-test-agent2")).To(Succeed())
		Expect(env.BootstrapNewAgent("monitoring-test-agent3")).To(Succeed())

		time.Sleep(1 * time.Second)

		mcc := env.ManagementClientConn()
		mgmtClient = managementv1.NewManagementClient(mcc)
		adminClient = cortexadmin.NewCortexAdminClient(mcc)
		opsClient = cortexops.NewCortexOpsClient(mcc)

		_ = adminClient
	})
	It("should install the Monitoring capability", func() {
		b.Navigate(webUrl + "/monitoring")

		By("confirming that the Monitoring capability is not installed")
		Eventually("div.body > div.not-enabled").Should(b.BeVisible())
		Expect("button.btn").To(b.HaveInnerText("Install"))

		By("clicking the Install button")
		b.Click("button.btn")

		By("confirming that the storage and grafana tabs are visible")
		Eventually("li#storage").Should(b.BeVisible())
		Eventually("li#grafana").Should(b.BeVisible())

		By("confirming that the storage tab is selected")
		Expect("section#storage").To(b.BeVisible())
		Expect("section#grafana").NotTo(b.BeVisible())

		By("selecting Standalone from the Mode dropdown")
		Eventually("div.v-select input.vs__search").Should(b.SetValue("Standalone"))
		Eventually("ul.vs__dropdown-menu > li").Should(b.Click())

		By("selecting Filesystem from the Storage Type dropdown")
		Eventually("section#storage div.v-select input.vs__search").Should(b.SetValue("Filesystem"))
		Eventually("ul.vs__dropdown-menu > li").Should(b.Click())

		By("clicking the Grafana tab")
		b.Click("li#grafana > a")

		By("confirming that the storage tab is hidden and the grafana tab is visible")
		Expect("section#storage").NotTo(b.BeVisible())
		Expect("section#grafana").To(b.BeVisible())

		By("confirming that the Grafana tab has a text box for the hostname")
		Eventually("section#grafana div.labeled-input input").Should(b.BeVisible())

		By("clicking the disable button")
		b.Click("section#grafana button.disable-button")

		By("clicking the install button")
		b.Click("div.resource-footer > button.role-primary")

		By("confirming that the Monitoring capability is installed")
		Eventually("div.banner").Should(And(b.BeVisible(), b.HaveInnerText("Monitoring is currently installed on the cluster.")))

		status, err := opsClient.Status(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.InstallState).To(Equal(driverutil.InstallState_Installed))
	})

	It("should show all agents in the Capability Management table", func() {
		By("confirming that the local agent has the Degraded badge")
		Eventually(Table().Row(1)).Should(MatchCells(CheckBox(), HaveDegradedBadge(), b.HaveInnerText("monitoring-test-agent1")))
		By("confirming that the other agents have the Not Installed badge")
		Eventually(Table().Row(2)).Should(MatchCells(CheckBox(), HaveNotInstalledBadge(), b.HaveInnerText("monitoring-test-agent2")))
		Eventually(Table().Row(3)).Should(MatchCells(CheckBox(), HaveNotInstalledBadge(), b.HaveInnerText("monitoring-test-agent3")))
	})

	It("should install the capability on all agents", func() {
		// Select all agents
		By("selecting all agents")
		b.Click(Table().Header().Col(1) + "label.checkbox-container > span.checkbox-custom")

		// Install button should be enabled, and all agents should be selected
		By("confirming that the install button is enabled and all agents are selected")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(And(b.BeEnabled(), b.BeVisible()))
		Eventually(".fixed-header-actions > .bulk > button.role-primary > span").Should(b.HaveInnerText("Install"))
		Eventually(".fixed-header-actions > .bulk > label.action-availability").Should(And(b.BeVisible(), b.HaveInnerText("3 selected")))

		// Click the install button
		By("clicking the install button")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(ClickWithMouseover())

		// Confirm all agents have the installed badge
		By("confirming that all agents have the installed badge")
		Eventually(Table().Row(1).Col(2)).Should(HaveInstalledBadge())
		Eventually(Table().Row(2).Col(2)).Should(HaveInstalledBadge())
		Eventually(Table().Row(3).Col(2)).Should(HaveInstalledBadge())

		By("confirming that all agents have the capability installed")
		c, err := mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: "monitoring-test-agent1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeTrue())

		c, err = mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: "monitoring-test-agent2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeTrue())

		c, err = mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: "monitoring-test-agent3"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeTrue())
	})

	It("should configure roles", func() {
		By("switching to the Roles tab")
		b.Navigate(webUrl + "/monitoring/rbac/roles")

		By("confirming that there are no roles in the table")
		Eventually(Table()).Should(HaveNoRows())

		By("clicking the Create button")
		b.Click("header > div.actions-container > a.btn")

		By("entering a name")
		Eventually(".labeled-input > input").Should(b.SetValue("test-role"))

		By("selecting the Match Labels tab")
		b.Click("li#matchLabels > a")
		Eventually("section#matchLabels").Should(b.BeVisible())

		By("clicking the Add Label button")
		b.Click("section#matchLabels > div.key-value button")

		By("entering a key and value")
		Eventually("section#matchLabels .key input").Should(b.SetValue("test-key"))
		Eventually("section#matchLabels .value input").Should(b.SetValue("test-value"))
		time.Sleep(600 * time.Millisecond) // these text boxes have a 500ms(??) debounce

		By("clicking the Save button")
		b.Click("div.resource-footer > button.role-primary")

		By("confirming that the role is in the table")
		Eventually(Table().Row(1).Col(2)).Should(b.HaveInnerText("test-role"))
		Eventually(Table().Row(1).Col(4)).Should(HaveBadge("bubble", "test-key=test-value"))

		By("confirming that the role exists")
		role, err := mgmtClient.GetRole(context.Background(), &corev1.Reference{Id: "test-role"})
		Expect(err).NotTo(HaveOccurred())
		Expect(role.MatchLabels.MatchLabels).To(HaveKeyWithValue("test-key", "test-value"))
	})

	It("should configure role bindings", func() {
		By("switching to the Role Bindings tab")
		b.Navigate(webUrl + "/monitoring/rbac/role-bindings")

		By("confirming that there are no role bindings in the table")
		Eventually(Table()).Should(HaveNoRows())

		By("clicking the Create button")
		b.Click("header > div.actions-container > a.btn")

		By("entering a name")
		Eventually("div.labeled-input > input").Should(b.SetValue("test-role-binding"))

		By("selecting a role")
		Eventually("div.v-select input.vs__search").Should(b.SetValue("test-role"))
		Eventually("ul.vs__dropdown-menu > li").Should(b.Click())

		By("clicking the Add Subject button")
		Expect("section#subjects div.footer > button").To(b.HaveInnerText("Add Subject"))
		b.Click("section#subjects div.footer > button")

		By("entering a subject name")
		Eventually("section#subjects div.box > div.value > input").Should(b.SetValue("test-subject"))
		time.Sleep(100 * time.Millisecond) // this text box has a 50ms debounce on input

		By("clicking the Save button")
		b.Click("div.resource-footer > button.role-primary")

		By("confirming that the role binding is in the table")
		Eventually(Table().Row(1)).Should(MatchCells(CheckBox(), b.HaveInnerText("test-role-binding"), HaveBadge("bubble", "test-subject"), b.HaveInnerText("test-role")))

		By("confirming that the role binding exists")
		roleBinding, err := mgmtClient.GetRoleBinding(context.Background(), &corev1.Reference{Id: "test-role-binding"})
		Expect(err).NotTo(HaveOccurred())
		Expect(roleBinding.Subjects).To(ConsistOf("test-subject"))
		Expect(roleBinding.RoleId).To(Equal("test-role"))
	})

	doQuery := func() (*http.Response, error) {
		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: env.GatewayClientTLSConfig(),
			},
		}
		gatewayAddr := env.GatewayConfig().Spec.HTTPListenAddress
		req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/api/prom/api/v1/query", gatewayAddr), nil)
		Expect(err).NotTo(HaveOccurred())
		q := req.URL.Query()
		q.Add("query", "up")
		req.URL.RawQuery = q.Encode()
		req.Header.Set("Authorization", "test-subject")
		return client.Do(req)
	}
	When("making queries using the new subject", func() {
		It("should not be able to access metrics from any clusters yet", func() {
			resp, err := doQuery()
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})
	When("labeling clusters to match the test role", func() {
		It("should be able to access metrics from the labeled clusters", func() {
			ids := []string{"monitoring-test-agent1", "monitoring-test-agent2", "monitoring-test-agent3"}
			for i, id := range ids {
				By("labeling cluster " + id)
				cluster, err := mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: id})
				Expect(err).NotTo(HaveOccurred())
				cluster.Metadata.Labels["test-key"] = "test-value"
				_, err = mgmtClient.EditCluster(context.Background(), &managementv1.EditClusterRequest{
					Cluster: cluster.Reference(),
					Labels:  cluster.Metadata.Labels,
				})
				Expect(err).NotTo(HaveOccurred())

				By("querying metrics")
				var vec *model.Vector
				Eventually(func() error {
					resp, err := doQuery()
					if err != nil {
						return err
					}
					if resp.StatusCode != http.StatusOK {
						return fmt.Errorf("expected status code 200, got %d", resp.StatusCode)
					}
					data, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						return err
					}
					qr, err := compat.UnmarshalPrometheusResponse(data)
					if err != nil {
						return err
					}
					if vec, err = qr.GetVector(); err != nil {
						return err
					}
					if len(*vec) != i+1 {
						return fmt.Errorf("expected %d samples, got %d: %v", i+1, len(*vec), vec.String())
					}
					return nil
				}, 60*time.Second, 500*time.Millisecond).Should(Succeed())

				expectedIds := ids[:i+1]
				actualIds := make([]string, 0, len(*vec))
				for _, sample := range *vec {
					actualIds = append(actualIds, string(sample.Metric["__tenant_id__"]))
				}
				Expect(actualIds).To(ConsistOf(expectedIds))
			}
		})
	})
	It("should delete the role binding", func() {
		b.Navigate(webUrl + "/monitoring/rbac/role-bindings")

		By("selecting the role binding")
		Eventually(Table().Row(1).Col(1) + "label.checkbox-container > span.checkbox-custom").Should(b.Click())

		By("confirming that the delete button is enabled and the single role binding is selected")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(And(b.BeEnabled(), b.BeVisible()))
		Eventually(".fixed-header-actions > .bulk > button.role-primary > span").Should(b.HaveInnerText("Delete"))
		Eventually(".fixed-header-actions > .bulk > label.action-availability").Should(And(b.BeVisible(), b.HaveInnerText("1 selected")))

		By("clicking the delete button")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(ClickWithMouseover())

		By("ensuring the delete modal is visible")
		Eventually("div.prompt-remove").Should(b.BeVisible())

		By("clicking the delete button")
		Eventually("div.prompt-remove div.card-actions > button.role-primary").Should(b.Click())

		By("ensuring the role binding is removed")
		Eventually(Table()).Should(HaveNoRows())
	})
	It("should delete the role", func() {
		b.Navigate(webUrl + "/monitoring/rbac/roles")

		By("selecting the role")
		Eventually(Table().Row(1).Col(1) + "label.checkbox-container > span.checkbox-custom").Should(b.Click())

		By("confirming that the delete button is enabled and the single role is selected")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(And(b.BeEnabled(), b.BeVisible()))
		Eventually(".fixed-header-actions > .bulk > button.role-primary > span").Should(b.HaveInnerText("Delete"))
		Eventually(".fixed-header-actions > .bulk > label.action-availability").Should(And(b.BeVisible(), b.HaveInnerText("1 selected")))

		By("clicking the delete button")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(ClickWithMouseover())

		By("ensuring the delete modal is visible")
		Eventually("div.prompt-remove").Should(b.BeVisible())

		By("clicking the delete button")
		Eventually("div.prompt-remove div.card-actions > button.role-primary").Should(b.Click())

		By("ensuring the role is removed")
		Eventually(Table()).Should(HaveNoRows())
	})

	It("should uninstall the capability on all agents", func() {
		By("navigating to the monitoring page")
		b.Navigate(webUrl + "/monitoring")

		By("selecting all agents")
		Eventually(Table().Header().Col(1) + "label.checkbox-container > span.checkbox-custom").Should(b.Click())

		By("confirming that the uninstall button is enabled and all agents are selected")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(And(b.BeEnabled(), b.BeVisible()))
		Eventually(".fixed-header-actions > .bulk > button.role-primary > span").Should(b.HaveInnerText("Uninstall"))
		Eventually(".fixed-header-actions > .bulk > label.action-availability").Should(And(b.BeVisible(), b.HaveInnerText("3 selected")))

		By("clicking the uninstall button")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(ClickWithMouseover())

		By("checking that the uninstall modal is visible")
		Eventually("div.uninstall-capabilities-dialog").Should(b.BeVisible())

		By("confirming that the Save button is disabled")
		Eventually("div.uninstall-capabilities-dialog .card-actions button.role-primary").Should(And(b.BeVisible(), Not(b.BeEnabled())))

		By(`entering "Monitoring" in the confirmation field`)
		Eventually("div.uninstall-capabilities-dialog input.no-label").Should(b.SetValue("Monitoring"))

		By("confirming that the Save button is enabled")
		Eventually("div.uninstall-capabilities-dialog .card-actions button.role-primary").Should(b.BeEnabled())

		By("clicking the Save button")
		b.Click("div.uninstall-capabilities-dialog .card-actions button.role-primary")

		By("confirming that the local agent has the Degraded badge")
		Eventually(Table().Row(1)).Should(MatchCells(CheckBox(), HaveDegradedBadge(), b.HaveInnerText("monitoring-test-agent1")))
		By("confirming that the other agents have the Not Installed badge")
		Eventually(Table().Row(2)).Should(MatchCells(CheckBox(), Or(HaveNotInstalledBadge(), HaveUninstallingBadge()), b.HaveInnerText("monitoring-test-agent2")))
		Eventually(Table().Row(3)).Should(MatchCells(CheckBox(), Or(HaveNotInstalledBadge(), HaveUninstallingBadge()), b.HaveInnerText("monitoring-test-agent3")))

		By("confirming that all agents have the capability uninstalled")
		c, err := mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: "monitoring-test-agent1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeFalse())

		c, err = mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: "monitoring-test-agent2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeFalse())

		c, err = mgmtClient.GetCluster(context.Background(), &corev1.Reference{Id: "monitoring-test-agent3"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics))).To(BeFalse())
	})

	It("should uninstall monitoring", func() {
		By("ensuring the uninstall button is visible")
		Eventually("header button.bg-error").Should(And(b.BeVisible(), b.HaveInnerText("Uninstall")))
		Eventually("header button.bg-error").Should(ClickWithMouseover())

		By("confirming that the Monitoring capability is not installed")
		Eventually("div.body > div.not-enabled").Should(b.BeVisible())
		Expect("button.btn").To(b.HaveInnerText("Install"))

		status, err := opsClient.Status(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.InstallState).To(Equal(driverutil.InstallState_NotInstalled))
	})

	It("should delete the agents", func() {
		By("navigating to the agents page")
		b.Navigate(webUrl + "/agents")

		By("selecting all agents")
		Eventually(Table().Header().Col(1) + "label.checkbox-container > span.checkbox-custom").Should(b.Click())

		By("confirming that the delete button is enabled and all agents are selected")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(And(b.BeEnabled(), b.BeVisible()))
		Eventually(".fixed-header-actions > .bulk > button.role-primary > span").Should(b.HaveInnerText("Delete"))
		Eventually(".fixed-header-actions > .bulk > label.action-availability").Should(And(b.BeVisible(), b.HaveInnerText("3 selected")))

		By("clicking the delete button")
		Eventually(".fixed-header-actions > .bulk > button.role-primary").Should(ClickWithMouseover())

		By("checking that the delete modal is visible")
		Eventually("div.prompt-remove").Should(b.BeVisible())

		By("confirming that the Delete button is enabled")
		Eventually("div.prompt-remove .card-actions button.role-primary").Should(And(b.BeVisible(), b.BeEnabled()))

		By("clicking the Delete button")
		b.Click("div.prompt-remove .card-actions button.role-primary")

		By("confirming that there are no agents in the table")
		Eventually(Table()).Should(HaveNoRows())

		By("confirming that the agents are deleted")
		l, err := mgmtClient.ListClusters(context.Background(), &managementv1.ListClustersRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(l.Items).To(BeEmpty())
	})
})
