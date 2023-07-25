package flagutil_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/rancher/opni/internal/cortex/config/validation"
	"github.com/rancher/opni/pkg/util/flagutil"
)

var _ = Describe("Flag Marshaling", Label("unit"), func() {
	var msg *validation.Limits
	BeforeEach(func() {
		msg = &validation.Limits{}
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
			msg.IngestionRate = lo.ToPtr[float64](1234567)
			msg.IngestionRateStrategy = lo.ToPtr("global")
			msg.RejectOldSamples = lo.ToPtr(true)
			msg.CreationGracePeriod = durationpb.New(1*time.Minute + 30*time.Second)
			msg.DropLabels = []string{"foo", "bar"}

			flags := flagutil.MarshalToFlags(msg)
			Expect(flags).To(ConsistOf(
				"--ingestion-rate=1.234567e+06",
				"--ingestion-rate-strategy=global",
				"--reject-old-samples=true",
				"--creation-grace-period=1m30s",
				"--drop-labels=foo,bar",
			))
		})
	})
})
