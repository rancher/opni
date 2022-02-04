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
var _ = XDescribe("Management API Cluster Management Tests", Ordered, func() {
	var environment *test.Environment
	var client management.ManagementClient
	BeforeAll(func() {
		fmt.Println("Starting test environment")
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
		// TODO: Create a cluster to be used by the tests
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})
	//#endregion

	//#region Happy Path Tests
	It("can get information about a specific cluster", func() {
		clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{})
		//TODO: What else is needed for this to work?
		Expect(err).NotTo(HaveOccurred())

		//TODO: Provide accurate assertions for each of the cluster's info items
		Expect(clusterInfo.Id).To(Equal(""))
	})

	It("can list all clusters using the same label", func() {
		clusterInfo, err := client.ListClusters(context.Background(), &management.ListClustersRequest{})
		//TODO: What else is needed for this to work?
		Expect(err).NotTo(HaveOccurred())

		//TODO: Provide accurate assertions for the items in the list
		Expect(clusterInfo.GetItems()).NotTo(BeNil())
	})

	It("can edit the label a cluster is using", func() {
		// clusterInfo, err := client.EditCluster()(context.Background(), &management.EditClusterRequest{})
		// //TODO: What else is needed for this to work?
		// Expect(err).NotTo(HaveOccurred())

		// //TODO: Provide accurate assertions for the items in the list
		// Expect(clusterInfo.GetItems()).To(Equal(""))
	})

	It("can delete individual clusters", func() {
		_, err := client.DeleteCluster(context.Background(), &core.Reference{})
		//TODO: How do I specify the cluster to be deleted?
		Expect(err).NotTo(HaveOccurred())

		clusterInfo, err := client.GetCluster(context.Background(), &core.Reference{})
		Expect(err).NotTo(HaveOccurred())

		for _, clusterID := range clusterInfo.Id {
			//TODO: Supply the Cluster ID for the cluster created in the BeforeAll below
			Expect(clusterID).NotTo(Equal(""))
		}
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion
})
