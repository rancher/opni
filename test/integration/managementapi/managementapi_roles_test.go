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
	"google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
var _ = Describe("Management API Roles Management Tests", Ordered, func() {
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

	When("creating a new role", func() {
		// var err error
		// role, err := client.CreateRole(context.Background(), &core.Role{})
		// Expect(err).NotTo(HaveOccurred())

		// //TODO: Capture variables for each of the role items returned

		It("can get information about all roles", func() {
			// roleInfo, err := client.GetRole(context.Background(), &core.Reference{})
			// Expect(err).NotTo(HaveOccurred())

			// type roleInfo struct {
			// 	roleInfo.Name string `json:""`
			// 	roleInfo.ClusterIDs string `json:""`
			// }

			// for _, roleItems := range roleInfo.RoleName {
			// 	sl[rolebindingItems] =
			// }

			// Expect(roleItems).NotTo(BeNil())
			// //TODO: Add some assertions
		})
	})

	It("can list all roles", func() {
		role, err := client.ListRoles(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		Expect(role).NotTo(BeNil())
	})

	It("can delete an existing role", func() {
		_, err := client.DeleteRole(context.Background(), &core.Reference{})
		//TODO: How do I specify the role to delete?
		Expect(err).NotTo(HaveOccurred())

		roleInfo, err := client.ListRoles(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		//TODO: Provide
		Expect(roleInfo).NotTo(ContainSubstring(""))
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion

})
