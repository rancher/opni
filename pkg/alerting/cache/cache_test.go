package cache_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/cache"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/message"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
)

var _ = Describe("Message caching for alerting", Label("unit"), Ordered, func() {
	When("we use a frquency based cache", func() {
		It("should cache alert manager messages while preserving opni metadata", func() {
			cache := cache.NewLFUMessageCache(50)
			uuid := uuid.New().String()
			alert := config.Alert{
				Status:   "Firing",
				StartsAt: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				EndsAt:   time.Date(2777, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					message.NotificationPropertyOpniUuid: uuid,
				},
				Annotations: map[string]string{
					message.NotificationContentHeader:       "test",
					message.NotificationContentSummary:      "some body",
					message.NotificationPropertyFingerprint: "opaque-fingerprint",
				},
			}
			By("verifying the object is cached")
			cache.Set(alertingv1.OpniSeverity_Critical, "key", alert)
			obj, ok := cache.Get(alertingv1.OpniSeverity_Critical, "key")
			Expect(ok).To(BeTrue())
			Expect(obj).ToNot(BeNil())
			By("preserving message contents")
			Expect(obj.Notification.Title).To(Equal("test"))
			Expect(obj.Notification.Body).To(Equal("some body"))
			By("preserving additional properties")
			Expect(obj.Notification.Properties[message.NotificationPropertyFingerprint]).To(Equal("opaque-fingerprint"))

			By("verifying last updated time is persisted")
			cache.Set(alertingv1.OpniSeverity_Critical, "key", alert)
			obj, ok = cache.Get(alertingv1.OpniSeverity_Critical, "key")
			Expect(ok).To(BeTrue())
			Expect(obj).ToNot(BeNil())
			Expect(obj.LastUpdatedAt.AsTime().UnixNano()).To(BeNumerically(">", obj.ReceivedAt.AsTime().UnixNano()))
		})

		It("should construct unique keys based on message metadata", func() {
			lfuCache := cache.NewLFUMessageCache(50)
			uuid1 := uuid.New().String()
			uuid2 := uuid.New().String()
			uniqScenarios := []cache.MessageMetadata{
				{
					IsAlarm:     true,
					Uuid:        uuid1,
					Fingerprint: "hello",
				},
				{
					IsAlarm:        false,
					Uuid:           uuid1,
					GroupDedupeKey: "hello",
				},
				{
					IsAlarm:        false,
					Uuid:           uuid2,
					GroupDedupeKey: "some-key1",
				},
				{
					IsAlarm:        false,
					Uuid:           uuid2,
					GroupDedupeKey: "some-key2",
				},
				{
					IsAlarm:     true,
					Uuid:        uuid2,
					Fingerprint: "some-fingerprint1",
				},
				{
					IsAlarm:     true,
					Uuid:        uuid2,
					Fingerprint: "some-fingerprint2",
				},
			}
			keys := []string{}
			for _, s := range uniqScenarios {
				keys = append(keys, lfuCache.Key(s))
			}
			keys = lo.Uniq(keys)
			Expect(keys).To(HaveLen(len(uniqScenarios)))
		})

		It("should return a set of partitioned key layers", func() {
			cache := cache.NewLFUMessageCache(50)
			mappedKeys := cache.PartitionedKeys()
			Expect(mappedKeys).ToNot(BeNil())
			Expect(lo.Keys(mappedKeys)).To(ConsistOf(
				lo.Map(lo.Keys(alertingv1.OpniSeverity_name), func(r int32, _ int) alertingv1.OpniSeverity {
					return alertingv1.OpniSeverity(r)
				}),
			))
			for _, keys := range mappedKeys {
				Expect(keys).ToNot(BeNil())
				Expect(keys).To(HaveLen(0))
			}
		})
	})
})
