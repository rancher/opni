package capabilities_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

func makeCluster(caps ...string) *corev1.Cluster {
	return &corev1.Cluster{
		Id: "test",
		Metadata: &corev1.ClusterMetadata{
			Capabilities: lo.Map(caps, util.Indexed(capabilities.Cluster)),
		},
	}
}

func putEvent(old, new *corev1.Cluster) storage.WatchEvent[*corev1.Cluster] {
	return storage.WatchEvent[*corev1.Cluster]{
		EventType: storage.WatchEventUpdate,
		Previous:  old,
		Current:   new,
	}
}

func deleteEvent(old *corev1.Cluster) storage.WatchEvent[*corev1.Cluster] {
	return storage.WatchEvent[*corev1.Cluster]{
		EventType: storage.WatchEventDelete,
		Current:   nil,
		Previous:  old,
	}
}

var _ = Describe("Capability Context", func() {
	When("no specific capabilities are given", func() {
		var eventC chan storage.WatchEvent[*corev1.Cluster]
		var ctx context.Context
		BeforeEach(func() {
			eventC = make(chan storage.WatchEvent[*corev1.Cluster])
			var ca context.CancelFunc
			ctx, ca = capabilities.NewContext[*corev1.ClusterCapability](context.Background(), eventC)
			DeferCleanup(ca)
		})
		It("should do nothing if no events are sent", func() {
			Consistently(ctx.Done()).ShouldNot(BeClosed())
		})
		It("should cancel if the cluster loses a capability", func() {
			eventC <- putEvent(makeCluster("foo", "bar"), makeCluster("foo"))
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrCapabilityNotFound))
		})
		It("should not cancel if the cluster gains a capability", func() {
			eventC <- putEvent(makeCluster("foo"), makeCluster("foo", "bar"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
		})
		It("should not cancel if the cluster has no change", func() {
			eventC <- putEvent(makeCluster("foo"), makeCluster("foo"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
		})
		It("should cancel if the cluster loses multiple capabilities", func() {
			eventC <- putEvent(makeCluster("foo", "bar", "baz"), makeCluster())
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrCapabilityNotFound))
		})
		It("should cancel if the cluster is deleted", func() {
			eventC <- deleteEvent(makeCluster("foo", "bar"))
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrObjectDeleted))
		})
		It("should cancel if the cluster is deleted with no capabilities", func() {
			eventC <- deleteEvent(makeCluster())
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrObjectDeleted))
		})
		It("should cancel if the event channel is closed", func() {
			close(eventC)
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrEventChannelClosed))
		})
	})
	When("specific capabilities are given", func() {
		var eventC chan storage.WatchEvent[*corev1.Cluster]
		BeforeEach(func() {
			eventC = make(chan storage.WatchEvent[*corev1.Cluster])
		})

		setup := func(caps ...string) (context.Context, context.CancelFunc) {
			return capabilities.NewContext(context.Background(), eventC, lo.Map(caps, util.Indexed(capabilities.Cluster))...)
		}

		It("should do nothing if no events are sent", func() {
			ctx, ca := setup("foo")
			DeferCleanup(ca)
			Consistently(ctx.Done()).ShouldNot(BeClosed())
		})
		It("should cancel if the cluster loses a watched capability", func() {
			ctx, ca := setup("foo")
			defer ca()
			eventC <- putEvent(makeCluster("foo", "bar"), makeCluster("bar"))
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrCapabilityNotFound))
		})
		It("should not cancel if the cluster loses a capability that is not watched", func() {
			ctx, ca := setup("foo")
			defer ca()
			eventC <- putEvent(makeCluster("foo", "bar"), makeCluster("foo"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
		})
		It("should not cancel if the cluster never had the watched capability", func() {
			ctx, ca := setup("baz")
			defer ca()
			eventC <- putEvent(makeCluster("bar"), makeCluster("foo", "bar"))
			eventC <- putEvent(makeCluster("foo", "bar"), makeCluster("foo"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
		})
		It("should cancel if the cluster gains and then loses the watched capability", func() {
			ctx, ca := setup("baz")
			defer ca()
			eventC <- putEvent(makeCluster("bar"), makeCluster("foo", "bar"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
			eventC <- putEvent(makeCluster("foo", "bar"), makeCluster("foo", "bar", "baz"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
			eventC <- putEvent(makeCluster("foo", "bar", "baz"), makeCluster("baz"))
			Consistently(ctx.Done()).ShouldNot(BeClosed())
			eventC <- putEvent(makeCluster("baz"), makeCluster())
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrCapabilityNotFound))
		})
		It("should cancel if the cluster is deleted", func() {
			ctx, ca := setup("foo")
			defer ca()
			eventC <- deleteEvent(makeCluster("foo", "bar"))
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrObjectDeleted))
		})
		It("should cancel if the event channel is closed", func() {
			ctx, ca := setup("foo")
			defer ca()
			close(eventC)
			Eventually(ctx.Done()).Should(BeClosed())
			Expect(ctx.Err()).To(MatchError(capabilities.ErrEventChannelClosed))
		})
	})
	It("should cancel if the parent context is canceled", func() {
		ctx, ca := context.WithCancel(context.Background())
		eventC := make(chan storage.WatchEvent[*corev1.Cluster])
		ctx2, ca2 := capabilities.NewContext[*corev1.ClusterCapability](ctx, eventC)
		defer ca2()
		ca()
		Eventually(ctx2.Done()).Should(BeClosed())
		Expect(ctx2.Err()).To(Equal(ctx.Err()))
	})
	It("should inherit deadlines and values from parent contexts", func() {
		eventC := make(chan storage.WatchEvent[*corev1.Cluster])
		ctx, ca := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer ca()
		type test string
		testKey := ("test")
		ctx = context.WithValue(ctx, testKey, "value")

		ctx2, ca2 := capabilities.NewContext[*corev1.ClusterCapability](ctx, eventC)
		defer ca2()
		d, ok := ctx.Deadline()
		d2, ok2 := ctx2.Deadline()
		Expect(d).To(Equal(d2))
		Expect(ok).To(Equal(ok2))

		v, ok := ctx.Value(testKey).(test)
		v2, ok2 := ctx2.Value(testKey).(test)
		Expect(v).To(Equal(v2))
	})
})
