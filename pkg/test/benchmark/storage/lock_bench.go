package benchmark_storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
	"golang.org/x/sync/errgroup"
)

func LockManagerBenchmark[T storage.LockManager](
	name string,
	lmFs future.Future[[]T],
) func() {
	var lg *slog.Logger
	return func() {
		var lms []storage.LockManager
		var ctx context.Context
		BeforeAll(func() {
			testruntime.IfCI(func() {
				Skip("skipping lock benchmark in CI")
			})
			for _, lm := range lmFs.Get() {
				lms = append(lms, lm)
			}
			lg = logger.NewPluginLogger().WithGroup("lock-bench")
			ctx = context.Background()
		})

		Context("acquiring and holds many locks should have a small footprint", func() {
			AfterAll(func() {
				f, err := os.Create(fmt.Sprintf("%s-locks.mem.pprof", name))
				Expect(err).To(Succeed())
				defer f.Close()
				runtime.GC()
				if err := pprof.WriteHeapProfile(f); err != nil {
					panic(err)
				}
			})

			Specify("within the same process", func() {
				f, perr := os.Create(fmt.Sprintf("%s-single.locks.cpu.pprof", name))
				if perr != nil {
					panic(perr)
				}
				pprof.StartCPUProfile(f)
				defer pprof.StopCPUProfile()

				lm := lms[0]
				experiment := gmeasure.NewExperiment("single process locks")
				AddReportEntry(experiment.Name, experiment)

				uuids := make([]string, 1000)
				for i := 0; i < len(uuids); i++ {
					uuids[i] = uuid.New().String()
				}

				experiment.Sample(func(idx int) {
					tcs := []int{100, 1000}

					for _, n := range tcs {
						locks := make([]storage.Lock, n)
						for i := 0; i < n; i++ {
							locks[i] = lm.NewLock(uuids[i])
						}
						experimentName := fmt.Sprintf("unique locks acquired %d", n)
						experiment.MeasureDuration(experimentName, func() {
							var eg errgroup.Group
							for i := 0; i < n; i++ {
								i := i
								eg.Go(func() error {
									_, err := locks[i].Lock(ctx)
									return err
								})
							}
							Expect(eg.Wait()).Should(Succeed())
						})

						experimentName = fmt.Sprintf("unique locks unlocked %d", n)
						experiment.MeasureDuration(experimentName, func() {
							for i := 0; i < n; i++ {
								Expect(locks[i].Unlock()).To(Succeed())
							}
						})
					}
				}, gmeasure.SamplingConfig{N: 20, Duration: time.Minute})
			})

			Specify("accross multiple processes", func() {
				f, perr := os.Create(fmt.Sprintf("%s-multiple.locks.cpu.pprof", name))
				if perr != nil {
					panic(perr)
				}
				pprof.StartCPUProfile(f)
				defer pprof.StopCPUProfile()

				experiment := gmeasure.NewExperiment("multi process locks")
				AddReportEntry(experiment.Name, experiment)

				experiment.Sample(func(idx int) {
					type Testcase struct {
						N int // number of processes
						n int // number of locks per process
					}
					tcs := []Testcase{
						// distributed evenly per process
						{N: 3, n: 100},
						{N: 5, n: 100},
						{N: 7, n: 100},
						// distributed evenly per process
						{N: 3, n: 1000},
						{N: 5, n: 1000},
						{N: 7, n: 1000},
					}
					for _, tc := range tcs {
						locks := []storage.Lock{}
						for i := 0; i < tc.n; i++ {
							repl := i % tc.N
							locks = append(locks, lms[repl].NewLock(uuid.New().String()))
						}

						experimentName := fmt.Sprintf("unique locks acquired %d x %d", tc.N, len(locks))
						experiment.MeasureDuration(experimentName, func() {
							var eg errgroup.Group
							for i := 0; i < len(locks); i++ {
								i := i
								eg.Go(func() error {
									_, err := locks[i].Lock(ctx)
									return err
								})
							}
							Expect(eg.Wait()).Should(Succeed())
						})

						experimentName = fmt.Sprintf("unique locks unlocked %d x %d", tc.N, len(locks))
						experiment.MeasureDuration(experimentName, func() {
							for i := 0; i < len(locks); i++ {
								Expect(locks[i].Unlock()).To(Succeed())
							}
						})
					}
				}, gmeasure.SamplingConfig{N: 20, Duration: time.Minute})
			})
		})

		Context(fmt.Sprintf("High contention %s locks should resolve reasonably quickly", name), func() {
			AfterAll(func() {
				f, err := os.Create(fmt.Sprintf("%s-contention.mem.pprof", name))
				if err != nil {
					panic(err)
				}
				defer f.Close()
				runtime.GC()
				if err := pprof.WriteHeapProfile(f); err != nil {
					panic(err)
				}
			})
			// ==
			Specify("within the same process", func() {
				testKey := "test"
				// == comment out this block for more accurate benchmarks
				f, perr := os.Create(fmt.Sprintf("%s-single.cpu.pprof", name))
				if perr != nil {
					panic(perr)
				}
				pprof.StartCPUProfile(f)
				defer pprof.StopCPUProfile()
				lm := lms[0]
				experiment := gmeasure.NewExperiment("single process contention")
				AddReportEntry(experiment.Name, experiment)

				experiment.Sample(func(idx int) {
					tcs := []int{10, 100}
					for _, n := range tcs {
						lockers := make([]storage.Lock, n)
						for i := 0; i < n; i++ {
							lockers[i] = lm.NewLock(testKey)
						}
						experimentName := fmt.Sprintf("exclusive transactions %d", n)
						experiment.MeasureDuration(experimentName, func() {
							var eg errgroup.Group
							for i := 0; i < n; i++ {
								i := i
								eg.Go(func() error {
									defer func() {
										if err := lockers[i].Unlock(); err != nil {
											lg.With(logger.Err(err)).Error("failed to unlock")
										}
									}()
									_, err := lockers[i].Lock(ctx)
									return err
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
				experiment := gmeasure.NewExperiment("multiple process conflicting")
				AddReportEntry(experiment.Name, experiment)

				experiment.Sample(func(idx int) {
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
								lockers[i*tc.n+j] = lms[i].NewLock("test")
							}
						}
						experiment.MeasureDuration(fmt.Sprintf("distributed exclusive transactions %dX%d", tc.N, tc.n), func() {
							var eg errgroup.Group
							for i := 0; i < len(lockers); i++ {
								i := i
								eg.Go(func() error {
									defer lockers[i].Unlock()
									_, err := lockers[i].Lock(ctx)
									return err
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
