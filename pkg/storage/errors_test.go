package storage_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ = Describe("Errors", Label("unit"), func() {
	Specify("ErrNotFound should be equal to grpc NotFound errors", func() {
		err := status.Errorf(codes.NotFound, "sample text") // important that the message is not "not found"
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeFalse())
		Expect(storage.IsNotFound(err)).To(BeTrue())

		err = status.Errorf(codes.Internal, "sample text")
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeFalse())
		Expect(storage.IsNotFound(err)).To(BeFalse())

		err = status.Convert(nil).Err()
		Expect(errors.Is(err, storage.ErrNotFound)).To(BeFalse())
		Expect(storage.IsNotFound(err)).To(BeFalse())

		stat, ok := status.FromError(storage.ErrNotFound)
		Expect(ok).To(BeTrue())
		Expect(stat.Code()).To(Equal(codes.NotFound))
		ok = storage.IsNotFound(stat.Err())
		Expect(ok).To(BeTrue())
	})
	Specify("IsAlreadyExists should match errors that have codes.AlreadyExists", func() {
		err := status.Errorf(codes.AlreadyExists, "sample text")
		Expect(errors.Is(err, storage.ErrAlreadyExists)).To(BeFalse())
		Expect(storage.IsAlreadyExists(err)).To(BeTrue())

		err = status.Errorf(codes.Internal, "sample text")
		Expect(errors.Is(err, storage.ErrAlreadyExists)).To(BeFalse())
		Expect(storage.IsAlreadyExists(err)).To(BeFalse())

		err = status.Convert(nil).Err()
		Expect(errors.Is(err, storage.ErrAlreadyExists)).To(BeFalse())
		Expect(storage.IsAlreadyExists(err)).To(BeFalse())

		stat, ok := status.FromError(storage.ErrAlreadyExists)
		Expect(ok).To(BeTrue())
		Expect(stat.Code()).To(Equal(codes.AlreadyExists))
		ok = errors.Is(stat.Err(), storage.ErrAlreadyExists)
		Expect(ok).To(BeTrue())
	})
	Specify("IsConflict should not match all errors that have codes.Aborted", func() {
		conflictErr := fmt.Errorf("%w: %s", storage.ErrConflict, "sample text")
		notConflictErr := status.New(codes.Aborted, "sample text").Err()

		Expect(errors.Is(conflictErr, storage.ErrConflict)).To(BeTrue())
		Expect(storage.IsConflict(conflictErr)).To(BeTrue())

		Expect(errors.Is(notConflictErr, storage.ErrConflict)).To(BeFalse())
		Expect(storage.IsConflict(notConflictErr)).To(BeFalse())

		conflictErr2 := status.New(codes.Aborted, "sample text")
		conflictErr2, _ = conflictErr2.WithDetails(storage.ErrDetailsConflict)
		Expect(errors.Is(conflictErr2.Err(), storage.ErrConflict)).To(BeFalse())
		Expect(storage.IsConflict(conflictErr2.Err())).To(BeTrue())

		stat, ok := status.FromError(storage.ErrConflict)
		Expect(ok).To(BeTrue())
		Expect(stat.Code()).To(Equal(codes.Aborted))
		ok = errors.Is(stat.Err(), storage.ErrConflict)
		Expect(ok).To(BeTrue())

		stat, ok = status.FromError(notConflictErr)
		Expect(ok).To(BeTrue())
		Expect(stat.Code()).To(Equal(codes.Aborted))
		ok = errors.Is(stat.Err(), storage.ErrConflict)
		Expect(ok).To(BeFalse())
	})
})
