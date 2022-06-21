package util_test

import (
	"errors"
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Generic Utils", Label("unit"), func() {
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
})
