package errors_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilerrors "github.com/rancher/opni/pkg/util/errors"
	"google.golang.org/grpc/codes"
)

var _ = Describe("Errors", Ordered, Label("unit"), func() {
	When("the errors are the same", func() {
		Specify("is should return true", func() {
			Expect(errors.Is(utilerrors.New(codes.Unknown, errTest), errTest)).To(BeTrue())
			Expect(errors.Is(utilerrors.New(codes.Unknown, errTest), utilerrors.New(codes.Unknown, errTest))).To(BeTrue())
		})
		Specify("as should return true", func() {
			testGRPC := utilerrors.New(codes.Unknown, errTest)
			Expect(errors.As(utilerrors.New(codes.Unknown, errTest), &testGRPC)).To(BeTrue())
		})
	})
	When("the errors are wrapped", func() {
		wrappedErr := fmt.Errorf("this is a wrapped err: %w", errTest)
		Specify("is should return true", func() {
			Expect(errors.Is(utilerrors.New(codes.Unknown, wrappedErr), errTest)).To(BeTrue())
			Expect(errors.Is(utilerrors.New(codes.Unknown, wrappedErr), utilerrors.New(codes.Unknown, errTest))).To(BeTrue())
		})
		Specify("as should return true", func() {
			testGRPC := utilerrors.New(codes.Unknown, errTest)
			Expect(errors.As(utilerrors.New(codes.Unknown, wrappedErr), &testGRPC)).To(BeTrue())
		})
	})
})
