package alerting_test

import (
	"path"
	"strconv"
	"time"

	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Internal alerting plugin functionality test", Ordered, Label(test.Unit, test.Slow), func() {
	BeforeAll(func() {
		// ...
		alerting.AlertPath = "alerttestdata/logs"
		err := os.RemoveAll(alerting.AlertPath)
		Expect(err).To(BeNil())
		err = os.MkdirAll(alerting.AlertPath, 0755)
		Expect(err).To(BeNil())
	})

	When("We use basic on disk persistence for alerting", func() {
		Specify("The parse helper should be robust (enough)", func() {
			inputStr := ""
			timestamp, number := alerting.Parse(inputStr)
			Expect(timestamp).To(Equal(""))
			Expect(number).To(Equal("0"))

			inputStr = "2022-31-07_1"
			timestamp, number = alerting.Parse(inputStr)
			Expect(timestamp).To(Equal("2022-31-07"))
			Expect(number).To(Equal("1"))
			_, err := time.Parse(alerting.TimeFormat, timestamp)
			Expect(err).To(Succeed())
		})

		Specify("The bucket info Construct helper should be robust (enough)", func() {
			b := alerting.BucketInfo{}
			b.ConditionId = uuid.New().String()
			b.Timestamp = time.Now().Format(alerting.TimeFormat)
			b.Number = 1
			expected := alerting.AlertPath + "/" + b.ConditionId + "/" + (b.Timestamp + alerting.Separator + strconv.Itoa(b.Number))
			Expect(b.Construct()).To(Equal(expected))
		})

		Specify("When no index exists for a condition, its methods should return errors", func() {
			b := alerting.BucketInfo{
				ConditionId: "test",
			}
			Expect(b.IsFull()).To(BeFalse())
			val, err := b.Size()
			Expect(err).To(HaveOccurred())
			Expect(val).To(Equal(int64(-1)))
			err = b.MostRecent()
			Expect(err).To(HaveOccurred())
		})

		It("Should be able to create a new index for a condition id", func() {
			newId := uuid.New().String()
			b := alerting.BucketInfo{
				ConditionId: newId,
			}
			err := b.Create()
			Expect(err).To(BeNil())
			today := time.Now().Format(alerting.TimeFormat)
			Expect(b.Construct()).To(Equal(path.Join(alerting.AlertPath, newId, (today + alerting.Separator + "0"))))
			Expect(b.IsFull()).To(BeFalse())
			val, err := b.Size()
			Expect(err).To(BeNil())
			Expect(val).To(Equal(int64(0)))

			createdIndices, err := alerting.GetIndices()
			Expect(err).To(Succeed())
			Expect(createdIndices).To(HaveLen(1))
			// Getting indices should set the index info to the most recent bucket, if it exists
			Expect(createdIndices).To(ContainElement(&b))

		})

		It("Should be able to append to the bucket for an existing index", func() {
			existing, err := alerting.GetIndices()
			Expect(err).To(Succeed())
			Expect(existing).To(HaveLen(1))
			b := existing[0]
			Expect(b.IsFull()).To(BeFalse())
			Expect(b.Size()).To(Equal(int64(0)))
			err = b.Append(&corev1.AlertLog{
				ConditionId: &corev1.Reference{Id: b.ConditionId},
			})
			Expect(err).To(Succeed())
			Expect(b.Size()).To(BeNumerically(">", 0))
		})

		It("Should be able to append to an existing index with no buckets", func() {
			// simulates when we delete old data, but the condition still exists
			existing, err := alerting.GetIndices()
			today := time.Now().Format(alerting.TimeFormat)
			Expect(err).To(Succeed())
			Expect(existing).To(HaveLen(1))

			newId := uuid.New().String()
			// _ := alerting.BucketInfo{
			// 	ConditionId: newId,
			// }
			err = alerting.CreateIndex(newId)
			Expect(err).To(BeNil())
			current, err := alerting.GetIndices()
			Expect(err).To(Succeed())
			Expect(current).To(HaveLen(2))

			var newestBucket *alerting.BucketInfo
			for _, b := range current {
				if b.ConditionId == newId {
					newestBucket = b
				}
			}
			Expect(newestBucket).ToNot(BeNil())
			Expect(newestBucket.Timestamp).To(Equal(""))
			Expect(newestBucket.Number).To(Equal(0))

			err = newestBucket.Append(&corev1.AlertLog{})
			Expect(err).To(BeNil())
			Expect(newestBucket.Number).To(Equal(0))
			Expect(newestBucket.Timestamp).To(Equal(today))
		})

		It("Should handle buckets filling up", func() {
			existing, err := alerting.GetIndices()
			Expect(err).To(Succeed())
			Expect(existing).To(HaveLen(2))
			b := existing[0]
			approxBytes := len(b.ConditionId)
			log := &corev1.AlertLog{
				ConditionId: &corev1.Reference{Id: b.ConditionId},
			}

			threshold := (alerting.BucketMaxSize / int64(approxBytes)) + int64(approxBytes)*2
			for i := int64(0); i < threshold; i++ {
				err = b.Append(log)
				Expect(err).To(Succeed())
			}
			Expect(b.Number).To(BeNumerically(">", 0))

			checkSame, err := alerting.GetIndices()
			Expect(err).To(Succeed())
			Expect(checkSame).To(HaveLen(2))
		})
	})

})
