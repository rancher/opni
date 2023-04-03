package management

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
)

var _ = Describe("ManagementWatcherHooks", Ordered, Label("unit"), func() {
	var hooks *ManagementWatcherHooks[*managementv1.WatchEvent]
	var count int

	BeforeAll(func() {
		hooks = NewManagementWatcherHooks[*managementv1.WatchEvent](context.Background())
		hooks.RegisterHook(func(event *managementv1.WatchEvent) bool {
			return event.Type == managementv1.WatchEventType_Updated
		}, func(ctx context.Context, event *managementv1.WatchEvent) error {
			count++
			return nil
		})
	})

	It("should ignore non-update events", func() {
		hooks.HandleEvent(&managementv1.WatchEvent{
			Type: managementv1.WatchEventType_Created,
		})
		Expect(count).To(Equal(0))

		hooks.HandleEvent(&managementv1.WatchEvent{
			Type: managementv1.WatchEventType_Deleted,
		})
		Expect(count).To(Equal(0))
	})

	It("should handle update events", func() {
		hooks.HandleEvent(&managementv1.WatchEvent{
			Type: managementv1.WatchEventType_Updated,
		})

		Expect(count).To(Equal(1))
	})
})
