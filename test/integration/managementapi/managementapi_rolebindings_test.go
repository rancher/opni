package integration_test

// import (
// 	"encoding/json"
// 	"fmt"

// 	"github.com/kralicky/opni-monitoring/pkg/logger"
// 	"github.com/kralicky/opni-monitoring/pkg/management"
// 	"github.com/kralicky/opni-monitoring/pkg/test"
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// )

// //#region Test Setup
// var _ = Describe("Management API Rolebinding Management Tests", Ordered, func() {
// 	var environment *test.Environment
// 	var client management.ManagementClient
// 	BeforeAll(func() {
// 		fmt.Println("Starting test environment")
// 		environment = &test.Environment{
// 			TestBin: "../../testbin/bin",
// 			Logger:  logger.New().Named("test"),
// 		}
// 		Expect(environment.Start()).To(Succeed())
// 		client = environment.NewManagementClient()
// 		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
// 	})

// 	AfterAll(func() {
// 		fmt.Println("Stopping test environment")
// 		Expect(environment.Stop()).To(Succeed())
// 	})
// 	//#endregion

// 	//#region Happy Path Tests

// 	//#endregion

// 	//#region Edge Case Tests

// 	//#endregion

// })
