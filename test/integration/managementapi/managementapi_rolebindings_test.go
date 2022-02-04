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
var _ = XDescribe("Management API Rolebinding Management Tests", Ordered, func() {
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

	When("creating a new rolebinding", func() {
		// var err error
		// rolebinding, err := client.CreateRoleBinding(context.Background(), &core.RoleBinding{})
		// Expect(err).NotTo(HaveOccurred())

		// //TODO: Capture variables for each of the rolebindings items returned

		It("can get information about all rolebindings", func() {
			// rolebindingInfo, err := client.GetRoleBinding(context.Background(), &core.Reference{})
			// Expect(err).NotTo(HaveOccurred())

			// type rolebindingInfo struct {
			// 	rolebindingInfo.Name string `json:""`
			// 	rolebindingInfo.RoleName string `json:""`
			// 	rolebindingInfo.Subjects string `json:""`
			// 	rolebindingInfo.Taints string `json:""`
			// }

			// for _, rolebindingItems := range rolebindingInfo.RoleName {
			// 	sl[rolebindingItems] =
			// }

			// Expect(rolebinding).NotTo(BeNil())
			// //TODO: Add some assertions
		})
	})

	It("can list all rolebindings", func() {
		rolebinding, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		Expect(rolebinding).NotTo(BeNil())
	})

	It("can delete an existing rolebinding", func() {
		_, err := client.DeleteRoleBinding(context.Background(), &core.Reference{})
		//TODO: How do I specify the rolebinding to delete?
		Expect(err).NotTo(HaveOccurred())

		rolebindingInfo, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		//TODO: Provide
		Expect(rolebindingInfo).NotTo(ContainSubstring(""))
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion

})
