package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Codecs", Label("unit"), func() {
	Context("DelimiterCodec", func() {
		It("should encode string slices with a delimiter", func() {
			codec := util.NewDelimiterCodec("key", ",")
			Expect(codec.Key()).To(Equal("key"))
			Expect(codec.Encode([]string{"key", "value"})).To(Equal("key,value"))
			Expect(codec.Encode([]string{"", "value"})).To(Equal("value"))
			Expect(codec.Encode([]string{" key", "value "})).To(Equal("key,value"))
		})
		It("should decode strings with a delimiter", func() {
			codec := util.NewDelimiterCodec("key", ",")
			Expect(codec.Key()).To(Equal("key"))
			Expect(codec.Decode("key,value")).To(Equal([]string{"key", "value"}))
			Expect(codec.Decode("value")).To(Equal([]string{"value"}))
			Expect(codec.Decode(" key,value ")).To(Equal([]string{"key", "value"}))
		})
	})
})
