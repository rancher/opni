package integration_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
var _ = Describe("Management API Cerificate Management Tests", Ordered, func() {
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
	It("can retrieve full certification chain information", func() {
		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		leaf := certsInfo.Chain[0]
		root := certsInfo.Chain[len(certsInfo.Chain)-1]

		Expect(root.Issuer).To(Equal("CN=Example Root CA"))
		Expect(root.Subject).To(Equal("CN=Example Root CA"))
		Expect(root.IsCA).To(BeTrue())
		Expect(root.Fingerprint).NotTo(BeEmpty())

		Expect(leaf.Issuer).To(Equal("CN=Example Root CA"))
		Expect(leaf.Subject).To(Equal("CN=leaf"))
		Expect(leaf.IsCA).To(BeFalse())
		Expect(leaf.Fingerprint).NotTo(BeEmpty())
	})

	//#endregion

	//#region Edge Case Tests

	//#endregion
})
