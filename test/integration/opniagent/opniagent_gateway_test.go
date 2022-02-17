package integration_test

import (
	// "context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// "github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	// "github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/test"
	// "google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
var _ = Describe("Opni Agent - Gateway Integration Tests", Ordered, func() {
	var environment *test.Environment
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests

	// var err error
	It("can output to Opni Gateway for a Single Cluster", func() {

	})

	It("can output to Opni Gateway for Multiple Clusters", func() {

	})

	//#endregion

	//#region Error Path Tests

	//TODO: Add error path tests

	//#endregion
})
