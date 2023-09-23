package management_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/management"
)

var _ = Describe("ManagementWatcherHooks", Ordered, Label("unit"), func() {
	var hooks *management.ManagementWatcherHooks[*managementv1.WatchEvent]
	var count int

	BeforeAll(func() {
		hooks = management.NewManagementWatcherHooks[*managementv1.WatchEvent](context.Background())
		hooks.RegisterHook(func(event *managementv1.WatchEvent) bool {
			return event.Type == managementv1.WatchEventType_Put
		}, func(ctx context.Context, event *managementv1.WatchEvent) error {
			count++
			return nil
		})
	})

	It("should ignore non-update events", func() {
		hooks.HandleEvent(&managementv1.WatchEvent{
			Type: managementv1.WatchEventType_Delete,
		})
		Expect(count).To(Equal(0))
	})

	It("should handle update events", func() {
		hooks.HandleEvent(&managementv1.WatchEvent{
			Type: managementv1.WatchEventType_Put,
		})

		Expect(count).To(Equal(1))
	})
})
