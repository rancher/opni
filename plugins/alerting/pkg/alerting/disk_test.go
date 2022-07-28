package alerting_test

import (
	"github.com/hashicorp/go-hclog"

	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Internal alerting plugin functionality test", Ordered, Label(test.Unit, test.Slow), func() {
	var lg hclog.Logger
	BeforeAll(func() {
		// ...
		alerting.AlertPath = "./alerttestdata/logs"
		err := os.RemoveAll(alerting.AlertPath)
		Expect(err).To(BeNil())
		err = os.MkdirAll(alerting.AlertPath, 0755)
		Expect(err).To(BeNil())
		lg = hclog.Logger(hclog.New(&hclog.LoggerOptions{}))
	})

	When("We use basic on disk persistence for alerting", func() {
		It("Should be able to retrieve buckets and create new buckets", func() {
			res, err := alerting.GetBuckets(lg)
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(0))

			err = alerting.CreateBucket()
			Expect(err).To(BeNil())

			res, err = alerting.GetBuckets(lg)
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(1))

			err = alerting.CreateBucket()
			Expect(err).To(BeNil())
			err = alerting.CreateBucket()
			Expect(err).To(BeNil())
			res, err = alerting.GetBuckets(lg)
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(3))
			// Verify info
			for _, bucket := range res {
				info, err := alerting.ParseNameToInfo(bucket, lg)
				Expect(err).To(Succeed())
				Expect(info.Path).ToNot(Equal(""))
				Expect(alerting.BucketIsFull(info.Path)).To(BeFalse())
				Expect(info.Timestamp).To(Equal(time.Now().Format(alerting.TimeFormat)))
			}
			// clean up
			err = os.RemoveAll(alerting.AlertPath)
			Expect(err).To(BeNil())
			err = os.MkdirAll(alerting.AlertPath, 0755)
			Expect(err).To(BeNil())
		})

		It("Should be able to persist alert logs to a bucket until it is full", func() {

		})
	})

})
