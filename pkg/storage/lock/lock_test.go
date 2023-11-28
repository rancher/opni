package lock_test

import (
	"fmt"
	"sync"
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

	When("using a lock scheduler", func() {
		It("should act as pseudo-mutex", func() {
			l := lock.NewLockScheduler()
			num := int32(0)
			var wg sync.WaitGroup
			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := l.Schedule(func() error {
						return nil
					}); err != nil {
						panic(err)
					}

					// safe path
					num += 1

					if err := l.Done(func() error {
						return nil
					}); err != nil {
						panic(err)
					}
				}()
			}
			wg.Wait()
			Expect(num).To(Equal(int32(100)))
		})
	})

})
