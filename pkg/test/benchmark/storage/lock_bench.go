package benchmark_storage

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/lock"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
	"golang.org/x/sync/errgroup"
)

func LockManagerBenchmark[T storage.LockManager](
	name string,
	lmFs future.Future[[]T],
) func() {
	return func() {
		var lms []T
		BeforeAll(func() {
			testruntime.IfCI(func() {
				Skip("skipping lock benchmark in CI")
			})
			lms = lmFs.Get()
		})

		Context(fmt.Sprintf("Acquiring and releasing %s exclusive locks should be efficient", name), func() {
			Specify("within the same process", func() {
				lm := lms[0]
				expirement := gmeasure.NewExperiment("single process conflicting")
				AddReportEntry(expirement.Name, expirement)

				expirement.Sample(func(idx int) {
					tcs := []int{10, 100}
					for _, n := range tcs {
						lockers := make([]storage.Lock, n)
						for i := 0; i < n; i++ {
							lockers[i] = lm.Locker("test", lock.WithExclusiveLock(), lock.WithAcquireTimeout(lock.DefaultTimeout), lock.WithMaxRetries(10000), lock.WithRetryDelay(1*time.Microsecond))
						}
						expirement.MeasureDuration(fmt.Sprintf("exclusive transactions %d", n), func() {
							var eg errgroup.Group
							for i := 0; i < n; i++ {
								i := i
								eg.Go(func() error {
									defer lockers[i].Unlock()
									return lockers[i].Lock()
								})
							}
							err := eg.Wait()
							Expect(err).NotTo(HaveOccurred())
						})
					}
				}, gmeasure.SamplingConfig{N: 20, Duration: time.Minute})
			})

			XSpecify("accross multiple processes", func() {
				expirement := gmeasure.NewExperiment("multiple process conflicting")
				AddReportEntry(expirement.Name, expirement)

				expirement.Sample(func(idx int) {
					type Testcase struct {
						N int // number of processes
						n int // number of locks per process
					}
					tcs := []Testcase{
						{N: 3, n: 10},
						{N: 5, n: 10},
						{N: 7, n: 10},
						{N: 3, n: 100},
						{N: 5, n: 100},
						{N: 7, n: 100},
					}
					for _, tc := range tcs {
						if len(lms) < tc.N {
							Fail("not enough lock managers instantiated for benchmark")
						}

						lockers := make([]storage.Lock, tc.N*tc.n)

						for i := 0; i < tc.N; i++ {
							for j := 0; j < tc.n; j++ {
								lockers[i*tc.n+j] = lms[i].Locker("test", lock.WithAcquireTimeout(lock.DefaultTimeout), lock.WithMaxRetries(1000), lock.WithRetryDelay(1*time.Microsecond))
							}
						}
						expirement.MeasureDuration(fmt.Sprintf("distributed exclusive transactions %dX%d", tc.N, tc.n), func() {
							var eg errgroup.Group
							for i := 0; i < len(lockers); i++ {
								i := i
								eg.Go(func() error {
									defer lockers[i].Unlock()
									return lockers[i].Lock()
								})
							}
							err := eg.Wait()
							Expect(err).NotTo(HaveOccurred())
						})
					}
				}, gmeasure.SamplingConfig{N: 20, Duration: time.Minute})
			})
		})

		XContext(fmt.Sprintf("Using %s mututal exclusion locks across processes", name), func() {
			Specify("within the same process", func() {
				expirement := gmeasure.NewExperiment("single process non-conflicting")
				AddReportEntry(expirement.Name, expirement)
				lm := lms[0]
				expirement.Sample(func(idx int) {
					tcs := []int{10, 100}
					for _, n := range tcs {
						lockers := make([]storage.Lock, n)
						for i := 0; i < n; i++ {
							lockers[i] = lm.Locker("test", lock.WithConcurrentRead(), lock.WithAcquireTimeout(lock.DefaultTimeout), lock.WithMaxRetries(1000), lock.WithRetryDelay(1*time.Microsecond))
						}
						expirement.MeasureDuration(fmt.Sprintf("lock %d", n), func() {
							var eg errgroup.Group
							for i := 0; i < n; i++ {
								i := i
								eg.Go(func() error {
									return lockers[i].Lock()
								})
							}
							err := eg.Wait()
							Expect(err).NotTo(HaveOccurred())
						})

						expirement.MeasureDuration(fmt.Sprintf("unlock %d", n), func() {
							var eg errgroup.Group
							for i := 0; i < n; i++ {
								i := i
								eg.Go(func() error {
									return lockers[i].Unlock()
								})
							}
							err := eg.Wait()
							Expect(err).NotTo(HaveOccurred())
						})
					}
				}, gmeasure.SamplingConfig{N: 20, Duration: time.Minute})
			})
			Specify("across multiple processes", func() {
				expirement := gmeasure.NewExperiment("multiple process non-conflicting")

				N := 7 // number of processes
				if len(lms) < N {
					Fail("not enough lock managers instantiated for benchmark")
				}

				n := 10 // number of locks per process
				lockers := make([]storage.Lock, N*n)

				for i := 0; i < N; i++ {
					for j := 0; j < n; j++ {
						lock := lms[i].Locker("test", lock.WithConcurrentRead(), lock.WithAcquireTimeout(lock.DefaultTimeout))
						lockers[i*n+j] = lock
					}
				}

				AddReportEntry(expirement.Name, expirement)

				expirement.Sample(func(idx int) {
					expirement.MeasureDuration(fmt.Sprintf("lock distributed %dX%d", N, n), func() {
						var eg errgroup.Group
						for i := 0; i < n*N; i++ {
							i := i
							eg.Go(func() error {
								return lockers[i].Lock()
							})
						}
						err := eg.Wait()
						Expect(err).NotTo(HaveOccurred())
					})

					expirement.MeasureDuration(fmt.Sprintf("unlock distributed %dX%d", N, n), func() {
						var eg errgroup.Group
						for i := 0; i < n*N; i++ {
							i := i
							eg.Go(func() error {
								return lockers[i].Unlock()
							})
						}
						err := eg.Wait()
						Expect(err).NotTo(HaveOccurred())
					})
				}, gmeasure.SamplingConfig{N: 20, Duration: time.Minute})
			})
		})
	}
}
