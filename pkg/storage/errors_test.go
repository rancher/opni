package storage_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = Describe("Errors", func() {
	Specify("ErrNotFound should be equal to grpc NotFound errors", func() {
		err := status.Errorf(codes.NotFound, "not found")
		ok := errors.Is(err, storage.ErrNotFound)
		Expect(ok).To(BeTrue())

		err = status.Errorf(codes.Internal, "internal")
		ok = errors.Is(err, storage.ErrNotFound)
		Expect(ok).To(BeFalse())

		err = status.Convert(nil).Err()
		ok = errors.Is(err, storage.ErrNotFound)
		Expect(ok).To(BeFalse())

		stat, ok := status.FromError(storage.ErrNotFound)
		Expect(ok).To(BeTrue())
		Expect(stat.Code()).To(Equal(codes.NotFound))
		ok = errors.Is(stat.Err(), storage.ErrNotFound)
		Expect(ok).To(BeTrue())
	})
	Specify("ErrAlreadyExists should be equal to grpc AlreadyExists errors", func() {
		err := status.Errorf(codes.AlreadyExists, "already exists")
		ok := errors.Is(err, storage.ErrAlreadyExists)
		Expect(ok).To(BeTrue())

		err = status.Errorf(codes.Internal, "internal")
		ok = errors.Is(err, storage.ErrAlreadyExists)
		Expect(ok).To(BeFalse())

		err = status.Convert(nil).Err()
		ok = errors.Is(err, storage.ErrAlreadyExists)
		Expect(ok).To(BeFalse())

		stat, ok := status.FromError(storage.ErrAlreadyExists)
		Expect(ok).To(BeTrue())
		Expect(stat.Code()).To(Equal(codes.AlreadyExists))
		ok = errors.Is(stat.Err(), storage.ErrAlreadyExists)
		Expect(ok).To(BeTrue())
	})
})
