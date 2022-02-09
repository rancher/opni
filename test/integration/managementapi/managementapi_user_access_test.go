package integration_test

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/test"
)

//#region Test Setup
var _ = XDescribe("Management API User/Subject Access Management Tests", Ordered, func() {
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
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests
	It("can return a list of all Cluster IDs that a specific User (Subject) can access", func() {
		accessList, err := client.SubjectAccess(context.Background(), &core.SubjectAccessRequest{
			Subject: "test-subject",
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(accessList.Items).To(HaveLen(1))
		//TODO: Add more assertions
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion
})
