package benchmark_storage

import (
	"fmt"
	"os"
	"runtime/pprof"
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

func LockManagerBenchmark[T storage.LockManagerBroker](
	name string,
	lmFs future.Future[[]T],
) func() {
	return func() {
		var lms []storage.LockManager
		BeforeAll(func() {
			testruntime.IfCI(func() {
				Skip("skipping lock benchmark in CI")
			})
			for _, lm := range lmFs.Get() {
				lms = append(lms, lm.LockManager("test"))
			}
		})

		Context(fmt.Sprintf("Acquiring and releasing %s exclusive locks should be efficient", name), func() {
			// == comment out this block for more accurate benchmarks
			f, perr := os.Create(fmt.Sprintf("%s-single.pprof", name))
			if perr != nil {
				panic(perr)
			}
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
			// ==
			Specify("within the same process", func() {
				lm := lms[0]
				expirement := gmeasure.NewExperiment("single process conflicting")
				AddReportEntry(expirement.Name, expirement)

				expirement.Sample(func(idx int) {
					tcs := []int{10, 100}
					for _, n := range tcs {
						lockers := make([]storage.Lock, n)
						for i := 0; i < n; i++ {
							lockers[i] = lm.Locker("test", lock.WithAcquireTimeout(lock.DefaultTimeout), lock.WithRetryDelay(1*time.Microsecond))
						}
						expirementName := fmt.Sprintf("exclusive transactions %d", n)
						expirement.MeasureDuration(expirementName, func() {
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

			Specify("accross multiple processes", func() {
				// == comment out this block for more accurate benchmarks
				f, perr := os.Create(fmt.Sprintf("%s-multiple.pprof", name))
				if perr != nil {
					panic(perr)
				}
				pprof.StartCPUProfile(f)
				defer pprof.StopCPUProfile()
				// ==
				expirement := gmeasure.NewExperiment("multiple process conflicting")
				AddReportEntry(expirement.Name, expirement)

				expirement.Sample(func(idx int) {
					type Testcase struct {
						N int // number of processes
						n int // number of locks per process
					}
					tcs := []Testcase{
						{N: 3, n: 3},
						{N: 5, n: 3},
						{N: 7, n: 3},
						// 100 distributed evenly per process
						{N: 3, n: 100 / 3},
						{N: 5, n: 100 / 5},
						{N: 7, n: 100 / 7},
					}
					for _, tc := range tcs {
						if len(lms) < tc.N {
							Fail("not enough lock managers instantiated for benchmark")
						}

						lockers := make([]storage.Lock, tc.N*tc.n)

						for i := 0; i < tc.N; i++ {
							for j := 0; j < tc.n; j++ {
								lockers[i*tc.n+j] = lms[i].Locker("test", lock.WithAcquireTimeout(lock.DefaultTimeout), lock.WithRetryDelay(1*time.Microsecond))
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
				}, gmeasure.SamplingConfig{N: 10, Duration: time.Minute})
			})
		})
	}
}
