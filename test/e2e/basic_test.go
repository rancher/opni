package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Basic Test", func() {
	It("should connect to the management server", func() {
		resp, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		fmt.Println(resp.String())
		Expect(err).NotTo(HaveOccurred())
	})
})
