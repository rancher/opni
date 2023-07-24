package lock_test

import (
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/lock"
)

var _ = Describe("Lock", Label("unit"), func() {
	When("using lock primtives", func() {
		It("should run the lock primtive once", func() {
			i := int32(0)
			l := lock.LockPrimitive{}
			l.Do(func() error {
				atomic.AddInt32(&i, 1)
				return nil
			})
			l.Do(func() error {
				atomic.AddInt32(&i, 1)
				return nil
			})
			Expect(i).To(Equal(int32(1)))
		})

		It("should return an error if the encapsulated function returns an error", func() {
			l := lock.LockPrimitive{}
			err := l.Do(func() error {
				return fmt.Errorf("test error")
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("test error"))
		})

		It("should err if a lock primitive is run twice", func() {
			l := lock.LockPrimitive{}
			err := l.Do(func() error {
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			err = l.Do(func() error {
				return nil
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(lock.ErrLockActionRequested))
		})
	})

	When("using opni lock pool", func() {
		It("should report not holding a lock if no such lock was ever registered", func() {
			l := lock.NewLockPool()
			Expect(l.Holds("test")).To(BeFalse())
		})

		It("should register a lock", func() {
			l := lock.NewLockPool()
			l.AddLock("test", 1)
			Expect(l.Holds("test")).To(BeTrue())
		})

		It("should register and remove a lock", func() {
			l := lock.NewLockPool()
			l.AddLock("test", 1)
			Expect(l.Holds("test")).To(BeTrue())
			l.RemoveLock("test", 1)
			Expect(l.Holds("test")).To(BeFalse())
		})

	})

})
