package util_test

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/mennanov/fmutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/protorand"
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
		It("should create field masks by presence", func() {
			rand := protorand.New[*ext.SampleMessage]()
			rand.Seed(GinkgoRandomSeed())
			obj, err := rand.GenPartial(0.5)
			Expect(err).NotTo(HaveOccurred())
			mask := util.NewFieldMaskByPresence(obj.ProtoReflect())
			fmutils.Prune(obj, mask.GetPaths())
			Expect(obj).To(testutil.ProtoEqual(&ext.SampleMessage{}))

			rand2 := protorand.New[*ext.SampleConfiguration]()
			rand2.Seed(GinkgoRandomSeed())
			obj2, err := rand2.GenPartial(0.5)
			Expect(err).NotTo(HaveOccurred())
			mask2 := util.NewFieldMaskByPresence(obj2.ProtoReflect())
			fmutils.Prune(obj2, mask2.GetPaths())
			Expect(obj2).To(testutil.ProtoEqual(&ext.SampleConfiguration{}))
		})
		It("should create complete field masks for a type", func() {
			mask := util.NewCompleteFieldMask[*ext.SampleMessage]()
			expected := &fieldmaskpb.FieldMask{
				Paths: []string{},
			}
			for i := 1; i <= 6; i++ {
				for j := 1; j <= i; j++ {
					expected.Paths = append(expected.Paths, fmt.Sprintf("field%d.field%d", i, j))
				}
			}
			for i, l := 0, len(expected.Paths); i < l; i++ {
				expected.Paths = append(expected.Paths, fmt.Sprintf("msg.%s", expected.Paths[i]))
			}
			expected.Normalize()
			Expect(mask).To(testutil.ProtoEqual(expected))

			mask2 := util.NewCompleteFieldMask[*ext.SampleConfiguration]()
			expected2 := &fieldmaskpb.FieldMask{
				Paths: []string{
					"revision.revision",
					"revision.timestamp.nanos",
					"revision.timestamp.seconds",
					"stringField",
					"secretField",
					"mapField",
					"repeatedField",
				},
			}
			for _, p := range expected.Paths {
				expected2.Paths = append(expected2.Paths, fmt.Sprintf("messageField.%s", p))
			}
			expected2.Normalize()
			Expect(mask2).To(testutil.ProtoEqual(expected2))
		})
	})

	Context("DeepCopy", func() {
		It("should copy a struct", func() {
			type testStruct struct {
				Field1 *string
				Field2 *int
			}
			ts := &testStruct{
				Field1: lo.ToPtr("test"),
				Field2: lo.ToPtr(1),
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
