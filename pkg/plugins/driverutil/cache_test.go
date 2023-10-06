package driverutil_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/samber/lo"
)

var _ = Describe("Cache", Label("unit"), func() {
	type driverStub int
	var driverCache driverutil.Cache[driverStub]
	var driverBuilders map[string]driverutil.Builder[driverStub]

	BeforeEach(func() {
		driverCache = driverutil.NewCache[driverStub]()
		driverBuilders = map[string]driverutil.Builder[driverStub]{}
		for i := 0; i < 100; i++ {
			db := func(ctx context.Context, opts ...driverutil.Option) (driverStub, error) {
				return driverStub(i), nil
			}
			driverBuilders[fmt.Sprintf("driver%d", i)] = db
		}
	})

	When("registering new drivers", func() {
		It("should be able to retrieve registered drivers by name", func() {
			for name, builder := range driverBuilders {
				driverCache.Register(name, builder)
			}

			for name, builder := range driverBuilders {
				registeredBuilder, ok := driverCache.Get(name)
				Expect(ok).To(BeTrue())

				expectedDriver, _ := builder(context.Background())
				Expect(reflect.ValueOf(registeredBuilder)).To(Equal(reflect.ValueOf(builder)))
				registeredDriver, _ := registeredBuilder(context.Background())
				Expect(registeredDriver).To(Equal(expectedDriver))
			}

			for name, builder := range driverBuilders {
				registeredBuilder, ok := driverCache.Get(name)
				Expect(ok).To(BeTrue())
				Expect(reflect.ValueOf(registeredBuilder)).To(Equal(reflect.ValueOf(builder)))
			}
		})
		When("registering a driver with the same name", func() {
			It("should overwrite the existing driver", func() {
				driverCache.Register("driver1", driverBuilders["driver1"])

				registeredBuilder, ok := driverCache.Get("driver1")
				Expect(ok).To(BeTrue())
				Expect(reflect.ValueOf(registeredBuilder)).To(Equal(reflect.ValueOf(driverBuilders["driver1"])))

				driverCache.Register("driver1", driverBuilders["driver2"])

				registeredBuilder, ok = driverCache.Get("driver1")
				Expect(ok).To(BeTrue())
				Expect(reflect.ValueOf(registeredBuilder)).To(Equal(reflect.ValueOf(driverBuilders["driver2"])))
			})
		})
	})

	When("unregistering drivers", func() {
		BeforeEach(func() {
			for name, builder := range driverBuilders {
				driverCache.Register(name, builder)
			}
		})

		It("should remove the driver from the cache", func() {
			var unregistered []string
			for name := range driverBuilders {
				driverCache.Unregister(name)
				unregistered = append(unregistered, name)

				d1, ok := driverCache.Get(name)
				Expect(ok).To(BeFalse())
				Expect(d1).To(BeNil())

				Expect(driverCache.List()).To(ConsistOf(lo.Without(lo.Keys(driverBuilders), unregistered...)))
			}
		})
	})

	It("should be thread-safe", func() {
		wg := sync.WaitGroup{}
		for name, builder := range driverBuilders {
			name, builder := name, builder
			wg.Add(1)
			go func() {
				defer wg.Done()
				driverCache.Register(name, builder)
			}()
		}
		wg.Wait()

		for name, builder := range driverBuilders {
			name, builder := name, builder
			wg.Add(1)
			go func() {
				defer wg.Done()
				b, ok := driverCache.Get(name)
				Expect(ok).To(BeTrue())
				Expect(reflect.ValueOf(b)).To(Equal(reflect.ValueOf(builder)))
			}()
		}
		wg.Wait()
	})
})
