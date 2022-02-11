package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/pkp"
	"github.com/rancher/opni-monitoring/pkg/test"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

//#region Test Setup
type fingerprintsData struct {
	TestData []fingerprintsTestData `json:"testData"`
}

type fingerprintsTestData struct {
	Cert         string             `json:"cert"`
	Fingerprints map[pkp.Alg]string `json:"fingerprints"`
}

var testFingerprints fingerprintsData
var _ = Describe("Management API Boostrap Token Management Tests", Ordered, func() {
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
	var token *core.BootstrapToken
	var fingerprint string
	It("can create a bootstrap token", func() {
		var err error
		token, err = client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{})
		Expect(err).NotTo(HaveOccurred())

		Expect(token.TokenID).NotTo(BeNil())
		Expect(utf8.RuneCountInString(token.TokenID)).To(Equal(12))
		Expect(token.LeaseID).NotTo(BeNil())
		Expect(utf8.RuneCountInString(token.Secret)).To(Equal(52))
		Expect(token.Secret).NotTo(BeNil())
		Expect(utf8.RuneCountInString(strconv.Itoa(int(token.LeaseID)))).To(Equal(19))
		Expect(token.Ttl).To(Equal(int64(120)))
	})

	It("can list all bootstrap tokens", func() {
		tokenList, err := client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		tokenInfo := tokenList.Items
		Expect(tokenInfo).To(HaveLen(1))
		for _, token := range tokenInfo {
			Expect(token.TokenID).NotTo(BeEmpty())
			Expect(token.Secret).NotTo(BeEmpty())
			Expect(token.Ttl).NotTo(BeZero())
			Expect(token.LeaseID).NotTo(BeZero())
		}
	})

	It("can revoke a bootstrap token", func() {
		_, err := client.RevokeBootstrapToken(context.Background(), token.Reference())
		Expect(err).NotTo(HaveOccurred())

		tokenList, err := client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		tokenInfo := tokenList.Items
		Expect(tokenInfo).To(BeEmpty())
	})

	//#endregion

	//#region Edge Case Tests

	When("an agent is added and there are no tokens", func() {
		It("should fail to bootstrap", func() {
			certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())

			_, errC := environment.StartAgent("foo", token, []string{fingerprint})
			Eventually(errC).Should(Receive(MatchError("bootsrap error: bootstrap failed: 405 Method Not Allowed")))
		})
	})

	It("cannot revoke a bootstrap token without specifying a Token ID", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.RevokeBootstrapToken(context.Background(), &core.Reference{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("NotFound desc = failed to get token: not found"))

		_, err = client.RevokeBootstrapToken(context.Background(), token.Reference())
		Expect(err).NotTo(HaveOccurred())
	})

	It("can create a bootstrap token with a specific TTL duration", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &management.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(token.TokenID).NotTo(BeNil())
		Expect(utf8.RuneCountInString(token.TokenID)).To(Equal(12))
		Expect(token.LeaseID).NotTo(BeNil())
		Expect(utf8.RuneCountInString(token.Secret)).To(Equal(52))
		Expect(token.Secret).NotTo(BeNil())
		Expect(utf8.RuneCountInString(strconv.Itoa(int(token.LeaseID)))).To(Equal(19))
		Expect(token.Ttl).To(Equal(int64(60)))

		_, err = client.RevokeBootstrapToken(context.Background(), token.Reference())
		Expect(err).NotTo(HaveOccurred())
	})

	//#endregion
})
