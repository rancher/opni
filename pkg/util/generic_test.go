package util_test

import (
	"context"
	"errors"
	"time"
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Generic Utils", Label(test.Unit), func() {
	Context("Must", func() {
		It("should panic if a non-nil error is given", func() {
			Expect(func() {
				util.Must("test", errors.New("test"))
			}).To(Panic())
			Expect(func() {
				util.Must(errors.New("test"))
			}).To(Panic())
			Expect(func() {
				util.Must(1, errors.New("test"), errors.New("test"))
			}).To(Panic())
			Expect(func() {
				util.Must(1, nil)
			}).NotTo(Panic())
		})
	})
	Context("DecodeStruct", func() {
		It("should decode a struct with json tags", func() {
			type testStruct struct {
				Field1 string `json:"field1"`
				Field2 int    `json:"field2"`
			}
			ts, err := util.DecodeStruct[testStruct](map[string]interface{}{
				"field1": "test",
				"field2": 1,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(ts.Field1).To(Equal("test"))
			Expect(ts.Field2).To(Equal(1))
		})
		It("should decode a struct with no tags", func() {
			type testStruct struct {
				Field1 string
				Field2 int
			}
			ts, err := util.DecodeStruct[testStruct](map[string]interface{}{
				"field1": "test",
				"field2": 1,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(ts.Field1).To(Equal("test"))
			Expect(ts.Field2).To(Equal(1))
		})
		It("should decode a struct with embedded structs", func() {
			type TestStruct struct {
				Field1 string
				Field2 int
			}
			type TestStruct2 struct {
				TestStruct `json:",squash"`
				Field3     string
			}
			ts, err := util.DecodeStruct[map[string]any](TestStruct2{
				TestStruct: TestStruct{
					Field1: "test",
					Field2: 1,
				},
				Field3: "test2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(*ts).To(Equal(map[string]any{
				"Field1": "test",
				"Field2": 1,
				"Field3": "test2",
			}))
		})
		It("should handle an invalid input type", func() {
			_, err := util.DecodeStruct[struct{}](1)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("DeepCopy", func() {
		It("should copy a struct", func() {
			type testStruct struct {
				Field1 *string
				Field2 *int
			}
			ts := &testStruct{
				Field1: util.Pointer("test"),
				Field2: util.Pointer(1),
			}
			ts2 := &testStruct{}
			util.DeepCopyInto(ts2, ts)
			Expect(ts2).To(Equal(ts))
			Expect(uintptr(unsafe.Pointer(ts.Field1))).NotTo(Equal(uintptr(unsafe.Pointer(ts2.Field1))))
			Expect(uintptr(unsafe.Pointer(ts.Field2))).NotTo(Equal(uintptr(unsafe.Pointer(ts2.Field2))))

			ts3 := util.DeepCopy(ts)
			Expect(ts3).To(Equal(ts))
			Expect(uintptr(unsafe.Pointer(ts.Field1))).NotTo(Equal(uintptr(unsafe.Pointer(ts3.Field1))))
			Expect(uintptr(unsafe.Pointer(ts.Field2))).NotTo(Equal(uintptr(unsafe.Pointer(ts3.Field2))))
		})
	})

	Context("Future", func() {
		Specify("Get should block until Set is called", func() {
			f := util.NewFuture[string]()
			go func() {
				time.Sleep(time.Millisecond * 100)
				f.Set("test")
			}()
			start := time.Now()
			Expect(f.Get()).To(Equal("test"))
			Expect(time.Since(start)).To(BeNumerically(">=", time.Millisecond*100))
		})
		Specify("GetContext should allow Get to be canceled", func() {
			f := util.NewFuture[string]()
			ctx, ca := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer ca()
			go func() {
				time.Sleep(time.Millisecond * 100)
				f.Set("test")
			}()
			start := time.Now()
			_, err := f.GetContext(ctx)
			Expect(err).To(MatchError(context.DeadlineExceeded))
			Expect(time.Since(start)).To(BeNumerically("~", time.Millisecond*50, time.Millisecond*10))

			f2 := util.NewFuture[string]()
			ctx, ca = context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer ca()
			go func() {
				time.Sleep(time.Millisecond * 25)
				f2.Set("test")
			}()
			start = time.Now()
			Expect(f2.GetContext(ctx)).To(Equal("test"))
			Expect(time.Since(start)).To(BeNumerically("~", time.Millisecond*25, time.Millisecond*10))
		})
	})
})
