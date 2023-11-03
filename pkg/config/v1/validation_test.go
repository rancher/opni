package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Gateway Config", func() {
	leafCertData := string(testdata.TestData("localhost.crt"))
	leafKeyData := string(testdata.TestData("localhost.key"))
	caCertData := string(testdata.TestData("root_ca.crt"))

	v := validation.MustNewValidator()
	DescribeTable("Validation",
		func(gatewayConfig proto.Message, expectedErr string) {
			err := v.Validate(gatewayConfig)
			if expectedErr == "" {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErr))
			}
		},
		Entry("Certs", &v1.CertsSpec{
			CaCertData:      &caCertData,
			ServingKeyData:  &leafKeyData,
			ServingCertData: &leafCertData,
		}, ""),
	)
})
