package v1_test

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

var _ = Describe("Value", Label("unit"), func() {
	sampleMessage := &ext.SampleConfiguration{
		MapField: map[string]string{
			"foo": "bar",
			"bar": "baz",
		},
		RepeatedField: []string{
			"foo",
			"bar",
		},
		EnumField: ext.SampleEnum_Foo.Enum(),
	}

	sampleList := sampleMessage.ProtoReflect().Get(sampleMessage.ProtoReflect().Descriptor().Fields().ByName("repeatedField")).List()
	sampleMap := sampleMessage.ProtoReflect().Get(sampleMessage.ProtoReflect().Descriptor().Fields().ByName("mapField")).Map()
	sampleEnum := sampleMessage.ProtoReflect().Get(sampleMessage.ProtoReflect().Descriptor().Fields().ByName("enumField")).Enum()

	DescribeTable("Upstream Conversion Tests",
		func(in protoreflect.Value, want any) {
			Expect(v1.NewValue(in).ToValue().Equal(in)).To(BeTrue(), "converted value should be equal to original")

			Expect(in.IsValid()).To(Equal(want != nil), "Value(%v).IsValid() = %v, want %v", in, in.IsValid(), want != nil)
			switch want := want.(type) {
			case int32:
				Expect(in.Int()).To(Equal(int64(want)), "Value(%v).Int() = %v, want %v", in, in.Int(), want)
			case int64:
				Expect(in.Int()).To(Equal(int64(want)), "Value(%v).Int() = %v, want %v", in, in.Int(), want)
			case uint32:
				Expect(in.Uint()).To(Equal(uint64(want)), "Value(%v).Uint() = %v, want %v", in, in.Uint(), want)
			case uint64:
				Expect(in.Uint()).To(Equal(uint64(want)), "Value(%v).Uint() = %v, want %v", in, in.Uint(), want)
			case float32:
				Expect(in.Float()).To(Equal(float64(want)), "Value(%v).Float() = %v, want %v", in, in.Float(), want)
			case float64:
				Expect(in.Float()).To(Equal(float64(want)), "Value(%v).Float() = %v, want %v", in, in.Float(), want)
			case string:
				Expect(in.String()).To(Equal(string(want)), "Value(%v).String() = %v, want %v", in, in.String(), want)
			case []byte:
				Expect(in.Bytes()).To(Equal(want), "Value(%v).Bytes() = %v, want %v", in, in.Bytes(), want)
			case protoreflect.EnumNumber:
				Expect(in.Enum()).To(Equal(want), "Value(%v).Enum() = %v, want %v", in, in.Enum(), want)
			case protoreflect.Message:
				Expect(in.Message()).To(Equal(want), "Value(%v).Message() = %v, want %v", in, in.Message(), want)
			case protoreflect.List:
				Expect(in.List()).To(Equal(want), "Value(%v).List() = %v, want %v", in, in.List(), want)
			case protoreflect.Map:
				Expect(in.Map()).To(Equal(want), "Value(%v).Map() = %v, want %v", in, in.Map(), want)
			}
		},
		Entry(nil, protoreflect.Value{}, nil),
		Entry(nil, protoreflect.ValueOf(nil), nil),
		Entry(nil, protoreflect.ValueOf(true), true),
		Entry(nil, protoreflect.ValueOf(int32(math.MaxInt32)), int32(math.MaxInt32)),
		Entry(nil, protoreflect.ValueOf(int64(math.MaxInt64)), int64(math.MaxInt64)),
		Entry(nil, protoreflect.ValueOf(uint32(math.MaxUint32)), uint32(math.MaxUint32)),
		Entry(nil, protoreflect.ValueOf(uint64(math.MaxUint64)), uint64(math.MaxUint64)),
		Entry(nil, protoreflect.ValueOf(float32(math.MaxFloat32)), float32(math.MaxFloat32)),
		Entry(nil, protoreflect.ValueOf(float64(math.MaxFloat64)), float64(math.MaxFloat64)),
		Entry(nil, protoreflect.ValueOf(string("hello")), string("hello")),
		Entry(nil, protoreflect.ValueOf([]byte("hello")), []byte("hello")),
		Entry(nil, protoreflect.ValueOf(sampleMessage.ProtoReflect()), sampleMessage.ProtoReflect()),
		Entry(nil, protoreflect.ValueOf(sampleList), sampleList),
		Entry(nil, protoreflect.ValueOf(sampleMap), sampleMap),
		Entry(nil, protoreflect.ValueOf(sampleEnum), sampleEnum),
	)
})
