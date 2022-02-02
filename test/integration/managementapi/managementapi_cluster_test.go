package integration_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//#region Test Setup
var _ = FDescribe("Management API Cluster Management Tests", Ordered, func() {
	var environment *test.Environment
	var client management.ManagementClient
	BeforeAll(func() {
		fmt.Println("Starting test environment")
		environment = &test.Environment{
			TestBin: "../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})
	//#endregion

	//#region Happy Path Tests
	It("can get information about a specific cluster", func() {
		clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.Id).To(Equal(""))
	})

	It("can list all clusters using the same label", func() {
		clusterInfo, err := client.ListClusters(context.Background(), &management.ListClustersRequest{})
		Expect(err).NotTo(HaveOccurred())

		Expect(clusterInfo.GetItems()).NotTo(BeNil())
	})

	It("can edit the label a cluster is using", func() {

	})

	//#endregion

	//#region Edge Case Tests

	//#endregion
})
