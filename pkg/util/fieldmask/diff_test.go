package fieldmask_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/util/fieldmask"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Diff", Label("unit"), func() {
	DescribeTable("identifying changes between protobuf messages",
		func(oldMsg, newMsg proto.Message, expectedPaths []string) {
			mask := fieldmask.Diff(oldMsg, newMsg)
			Expect(mask.Paths).To(ConsistOf(expectedPaths))
		},
		Entry("no changes",
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 1}},
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 1}},
			[]string{},
		),
		Entry("single field change",
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 1}},
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 2}},
			[]string{"field1.field1"},
		),
		Entry("multiple fields change",
			&ext.SampleMessage{
				Field1: &ext.Sample1FieldMsg{Field1: 1},
				Field2: &ext.Sample2FieldMsg{Field1: 2, Field2: 3},
			},
			&ext.SampleMessage{
				Field1: &ext.Sample1FieldMsg{Field1: 10},
				Field2: &ext.Sample2FieldMsg{Field1: 2, Field2: 30},
			},
			[]string{"field1.field1", "field2.field2"},
		),
		Entry("multiple fields change",
			&ext.Sample6FieldMsg{
				Field1: 1,
				Field2: 2,
				Field3: 3,
				Field4: 4,
				Field5: 5,
				Field6: 6,
			},
			&ext.Sample6FieldMsg{
				Field1: 10,
				Field2: 20,
				Field3: 30,
				Field4: 40,
				Field5: 5,
				Field6: 6,
			},
			[]string{"field1", "field2", "field3", "field4"},
		),
		Entry("nested message change",
			&ext.SampleMessage{Msg: &ext.SampleMessage2{Field1: &ext.Sample1FieldMsg{Field1: 1}}},
			&ext.SampleMessage{Msg: &ext.SampleMessage2{Field1: &ext.Sample1FieldMsg{Field1: 2}}},
			[]string{"msg.field1.field1"},
		),
		Entry("nested message add",
			&ext.SampleMessage{Msg: &ext.SampleMessage2{Field2: &ext.Sample2FieldMsg{Field1: 1}}},
			&ext.SampleMessage{Msg: &ext.SampleMessage2{Field2: &ext.Sample2FieldMsg{Field1: 2, Field2: 3}}},
			[]string{"msg.field2.field1", "msg.field2.field2"},
		),
		Entry("field added",
			&ext.SampleMessage{},
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 1}},
			[]string{"field1"},
		),
		Entry("field removed",
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 1}},
			&ext.SampleMessage{},
			[]string{"field1"},
		),
		Entry("field removed",
			&ext.SampleMessage{Field1: &ext.Sample1FieldMsg{Field1: 0}},
			&ext.SampleMessage{},
			[]string{"field1"},
		),
		Entry("repeated field change",
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b", "c"}},
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b", "d"}},
			[]string{"repeatedField"},
		),
		Entry("repeated field change",
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b", "c"}},
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b"}},
			[]string{"repeatedField"},
		),
		Entry("repeated field change",
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b", "c"}},
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b", "c", "d"}},
			[]string{"repeatedField"},
		),
		Entry("repeated field change",
			&ext.SampleConfiguration{RepeatedField: []string{"a", "b", "c"}},
			&ext.SampleConfiguration{RepeatedField: nil},
			[]string{"repeatedField"},
		),
	)

	It("should handle nil messages", func() {
		Expect(fieldmask.Diff[*ext.SampleMessage](nil, nil)).To(BeNil())
	})

	It("should panic on different message types", func() {
		Expect(func() {
			fieldmask.Diff[proto.Message](proto.Message(&ext.SampleMessage{}), proto.Message(&ext.SampleMessage2{}))
		}).To(Panic())
	})
})
