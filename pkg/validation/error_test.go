package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/validation"
)

var _ = Describe("Error", Label("unit"), func() {
	It("should convert to a GRPC error", func() {
		err := validation.Error("foo")
		Expect(err.Error()).To(Equal("foo"))
		Expect(status.Convert(err).Message()).To(Equal("foo"))
		Expect(status.Convert(err).Code()).To(Equal(codes.InvalidArgument))
	})
})
