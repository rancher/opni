package flagutil_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/util/flagutil"
)

var _ = Describe("Flag Marshaling", Label("unit"), func() {
	var msg *ext.BazRequest
	BeforeEach(func() {
		msg = &ext.BazRequest{}
		flagutil.LoadDefaults(msg)
	})
	When("there are no changes", func() {
		It("should return no flags", func() {
			flags := flagutil.MarshalToFlags(msg)
			Expect(flags).To(BeEmpty())
		})
	})
	When("fields are changed", func() {
		It("should return flags for the modified fields", func() {
			msg.ParamFloat64 = 1234567
			msg.ParamString = "asdf"
			msg.ParamBool = true
			msg.ParamDuration = durationpb.New(1*time.Minute + 30*time.Second)
			msg.ParamRepeatedString = []string{"foo", "bar"}

			flags := flagutil.MarshalToFlags(msg)
			Expect(flags).To(ConsistOf(
				"--param-float-64=1.234567e+06",
				"--param-string=asdf",
				"--param-bool=true",
				"--param-duration=1m30s",
				"--param-repeated-string=foo,bar",
			))
		})
	})
})
