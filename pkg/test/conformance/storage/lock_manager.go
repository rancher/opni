package conformance_storage

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"golang.org/x/sync/errgroup"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/samber/lo"
)

func LockManagerTestSuite(
	lmF future.Future[storage.LockManager],
	// each lm is expected to have a separate client connection for each
	lmSetF future.Future[lo.Tuple3[
		storage.LockManager, storage.LockManager, storage.LockManager,
	]],
) func() {
	return func() {
		var lm storage.LockManager
		var lmSet lo.Tuple3[storage.LockManager, storage.LockManager, storage.LockManager]
		var ctx context.Context

		BeforeAll(func() {
			ctxca, ca := context.WithCancel(context.Background())
			DeferCleanup(func() {
				ca()
			})
			ctx = ctxca
			lm = lmF.Get()
			lmSet = lmSetF.Get()
		})

		When("using exclusive distributed locks within the same client conn", func() {
			It("should lock and unlock locks of the same type", func() {
				lock1 := lm.Locker("todo")
				done1, err := lock1.Lock(ctx)
				Expect(err).To(Succeed())
				Expect(lock1.Unlock()).To(Succeed())
				Eventually(done1).Should(Receive())
				lock2 := lm.Locker("todo")
				done2, err := lock2.Lock(ctx)
				Expect(err).To(Succeed())
				Expect(lock2.Unlock()).To(Succeed())
				Eventually(done2).Should(Receive())
			})

			Specify("try lock should fail quickly if the lock is already held", func() {
				lock1 := lm.Locker("held")
				lock2 := lm.Locker("held")
				done1, err := lock1.Lock(ctx)
				Expect(err).To(Succeed())
				ack, done2, err := lock2.TryLock(ctx)
				Expect(err).To(Succeed())
				Expect(ack).To(BeFalse())
				Expect(done2).To(BeNil())
				Expect(lock1.Unlock()).To(Succeed())
				Eventually(done1).Should(Receive())
			})

			Specify("acquiring blocking locks should be cancellable", func() {
				lock1 := lm.Locker("block")
				lock2 := lm.Locker("block")

				done1, err := lock1.Lock(ctx)
				Expect(err).To(Succeed())
				ctxca, ca := context.WithCancel(ctx)
				defer ca()
				var eg errgroup.Group
				eg.Go(func() error {
					_, err := lock2.Lock(ctxca)
					return err
				})
				ca()
				err = eg.Wait()
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(context.Canceled))
				Expect(lock1.Unlock()).To(Succeed())
				Eventually(done1).Should(Receive())
			})

			// TODO : jetstream is slow to release locks (it is having the server time them out)
			It("should resolve concurrent lock requests", func() {
				locks := []lockWithTransaction{}
				for i := 0; i < 3; i++ {
					lock := lm.Locker("rarrgh")
					locks = append(locks, lockWithTransaction{
						A: lock,
						C: make(chan struct{}),
					})
				}
				num := 0
				errs := make(chan error, 2*len(locks))
				doneV := make(chan chan struct{}, 2*len(locks))
				lockOrder := lo.Shuffle(locks)
				var wg sync.WaitGroup
				for _, lock := range lockOrder {
					lock := lock
					wg.Add(1)
					go func() {
						defer func() {
							wg.Done()
						}()
						done, err := lock.A.Lock(ctx)
						num += 1
						sendWithJitter(lock.C)
						errs <- err
						doneV <- done
					}()
				}
				for _, lock := range lockOrder {
					lock := lock
					wg.Add(1)
					go func() {
						defer func() {
							wg.Done()
						}()
						<-lock.C
						err := lock.A.Unlock()
						errs <- err
					}()
				}
				wg.Wait()
				close(errs)
				close(doneV)
				for err := range errs {
					Expect(err).NotTo(HaveOccurred())
				}
				for done := range doneV {
					Eventually(done).Should(Receive())
				}
				Expect(num).To(Equal(len(locks)))
			})

			It("is 'safe' to reuse a lock", func() {
				lock1 := lm.Locker("todo")
				done1, err := lock1.Lock(ctx)
				Expect(err).To(Succeed())
				Expect(lock1.Unlock()).To(Succeed())
				Eventually(done1).Should(Receive())
				done2, err := lock1.Lock(ctx)
				Expect(err).To(Succeed())
				Expect(lock1.Unlock()).To(Succeed())
				Eventually(done2).Should(Receive())
			})

			It("is 'safe' to discard a lock's expired chan", func() {
				lock1 := lm.Locker("todo2")
				_, err := lock1.Lock(ctx)
				Expect(err).To(Succeed())
				Expect(lock1.Unlock()).To(Succeed())
				_, err = lock1.Lock(ctx)
				Expect(err).To(Succeed())
				Expect(lock1.Unlock()).To(Succeed())
			})

			It("is concurrently safe to do lock operations", func() {
				lock := lm.Locker("concurrent")
				var eg errgroup.Group
				num := 0

				for i := 0; i < 10; i++ {
					eg.Go(func() error {
						_, err := lock.Lock(ctx)
						if err != nil {
							return err
						}
						num++
						return lock.Unlock()
					})
				}

				Expect(eg.Wait()).To(Succeed())
				Expect(num).To(Equal(10))
			})

			When("using distributed locks across multiple client conns", func() {
				It("should be able to lock and unlock locks", func() {
					x := lmSet.A.Locker("todo")
					y := lmSet.B.Locker("todo")
					z := lmSet.C.Locker("todo")

					doneX, err := x.Lock(ctx)
					Expect(err).To(Succeed())
					Expect(x.Unlock()).To(Succeed())
					Eventually(doneX).Should(Receive())

					doneY, err := y.Lock(ctx)
					Expect(err).To(Succeed())
					Expect(y.Unlock()).To(Succeed())
					Eventually(doneY).Should(Receive())

					doneZ, err := z.Lock(ctx)
					Expect(err).To(Succeed())
					Expect(z.Unlock()).To(Succeed())
					Eventually(doneZ).Should(Receive())
				})

				Specify("acquiring blocking locks should be cancellable", func() {
					x := lmSet.A.Locker("block")
					y := lmSet.B.Locker("block")
					z := lmSet.C.Locker("block")

					doneZ, err := z.Lock(ctx)
					Expect(err).To(Succeed())

					ctxca, ca := context.WithCancel(ctx)
					defer ca()
					var eg util.MultiErrGroup
					eg.Go(func() error {
						_, err := x.Lock(ctxca)
						return err
					})

					eg.Go(func() error {
						_, err := y.Lock(ctxca)
						return err
					})
					ca()
					eg.Wait()
					for _, err := range eg.Errors() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(context.Canceled))
					}
					Expect(z.Unlock()).To(Succeed())
					Eventually(doneZ).Should(Receive())
				})

				Specify("try lock should fail quickly is another manager is holding a lock", func() {
					x := lmSet.A.Locker("qu")
					y := lmSet.B.Locker("qu")
					z := lmSet.C.Locker("qu")

					doneY, err := y.Lock(ctx)
					Expect(err).To(Succeed())

					ack, doneX, err := x.TryLock(ctx)
					Expect(err).To(Succeed())
					Expect(ack).To(BeFalse())
					Expect(doneX).To(BeNil())

					ack, doneZ, err := z.TryLock(ctx)
					Expect(err).To(Succeed())
					Expect(ack).To(BeFalse())
					Expect(doneZ).To(BeNil())

					Expect(y.Unlock()).To(Succeed())
					Eventually(doneY).Should(Receive())
				})
			})

			Context("others", func() {
				Specify("calling 'unlock' on a lock that was never acquired should error", func() {
					lock := lm.Locker("todo")
					unlockErr := lock.Unlock()
					Expect(unlockErr).To(HaveOccurred())
					// Expect(unlockErr).To(MatchError(storage.ErrUnlockUnheldLock))
				})

				Specify("it should not leak memory", func() {
					var start, end runtime.MemStats

					experiment := gmeasure.NewExperiment("memory test")
					AddReportEntry(experiment.Name, experiment)

					runtime.GC()
					runtime.ReadMemStats(&start)

					experiment.RecordValue("start", float64(start.Alloc/1024))

					num := 200
					locker := lm.Locker("membench")
					for i := 0; i < num; i++ {
						_, err := locker.Lock(context.Background())
						Expect(err).To(Succeed())
						err = locker.Unlock()
						Expect(err).To(Succeed())
					}
					runtime.GC()
					runtime.ReadMemStats(&end)
					experiment.RecordValue("end", float64(end.Alloc/1024))
				})
			})
		})
	}
}

type lockWithTransaction lo.Tuple3[storage.Lock, *transaction, chan struct{}]

type transaction struct {
	Uuid string
}

func jitter() time.Duration {
	rand.NewSource(time.Now().UnixNano())
	randomMicroseconds := rand.Intn(101)
	jitterInterval := time.Duration(randomMicroseconds) * time.Microsecond
	return jitterInterval
}

func sendWithJitter(send chan struct{}) {
	<-time.After(jitter())
	send <- struct{}{}
}
