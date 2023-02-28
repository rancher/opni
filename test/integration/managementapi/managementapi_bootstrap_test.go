package integration_test

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
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
var _ = Describe("Management API Boostrap Token Management Tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var client managementv1.ManagementClient
	BeforeAll(func() {
		environment = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(environment.Start()).To(Succeed())
		client = environment.NewManagementClient()
		Expect(json.Unmarshal(test.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	})

	AfterAll(func() {
		Expect(environment.Stop()).To(Succeed())
	})

	//#endregion

	//#region Happy Path Tests
	var token *corev1.BootstrapToken
	var fingerprint string
	It("can create a bootstrap token", func() {
		var err error
		token, err = client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(token.TokenID).NotTo(BeNil())
		Expect(utf8.RuneCountInString(token.TokenID)).To(Equal(12))
		Expect(utf8.RuneCountInString(token.Secret)).To(Equal(52))
		Expect(token.Secret).NotTo(BeNil())
		Expect(token.Metadata.Ttl).To(Equal(int64(time.Hour.Seconds())))
	})

	It("can get information about a specific token", func() {
		tokenInfo, err := client.GetBootstrapToken(context.Background(), &corev1.Reference{
			Id: token.TokenID,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(tokenInfo.TokenID).To(Equal(token.TokenID))
		Expect(tokenInfo.Secret).To(Equal(token.Secret))
		Expect(tokenInfo.Metadata.Ttl).Should(BeNumerically("<=", token.Metadata.Ttl))
		Expect(tokenInfo.Metadata.LeaseID).To(Equal(token.Metadata.LeaseID))
	})

	It("can list all bootstrap tokens", func() {
		tokenList, err := client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		tokenInfo := tokenList.Items
		Expect(tokenInfo).To(HaveLen(1))
		for _, token := range tokenInfo {
			Expect(token.TokenID).NotTo(BeEmpty())
			Expect(token.Secret).NotTo(BeEmpty())
			Expect(token.Metadata.Ttl).NotTo(BeZero())
			Expect(token.Metadata.LeaseID).NotTo(BeZero())
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
			Eventually(errC).Should(Receive(WithTransform(util.StatusCode, Equal(codes.Unavailable))))
		})
	})

	It("cannot revoke a bootstrap token without specifying a Token ID", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = client.RevokeBootstrapToken(context.Background(), &corev1.Reference{
			Id: "nonexistent",
		})
		Expect(err).To(HaveOccurred())
		Expect(status.Convert(err).Code()).To(Equal(codes.NotFound))

		_, err = client.RevokeBootstrapToken(context.Background(), token.Reference())
		Expect(err).NotTo(HaveOccurred())
	})

	It("can create a bootstrap token with a specific TTL duration", func() {
		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Minute),
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(token.TokenID).NotTo(BeNil())
		Expect(utf8.RuneCountInString(token.TokenID)).To(Equal(12))
		Expect(utf8.RuneCountInString(token.Secret)).To(Equal(52))
		Expect(token.Secret).NotTo(BeNil())
		Expect(token.Metadata.Ttl).To(Equal(int64(60)))

		_, err = client.RevokeBootstrapToken(context.Background(), token.Reference())
		Expect(err).NotTo(HaveOccurred())
	})

	//#endregion
})
