package notifier_test

import (
	"context"
	"sync"
	"time"

	"github.com/kralicky/gpkg/sync/atomic"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/model/rulefmt"
	mock_rules "github.com/rancher/opni/pkg/test/mock/rules"
	"github.com/samber/lo"

	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util/notifier"
)

var _ = Describe("Update Notifier", Label("unit"), func() {
	testGroups1 := []rules.RuleGroup{
		{
			Name:  "foo",
			Rules: []rulefmt.RuleNode{},
		},
		{
			Name:  "bar",
			Rules: []rulefmt.RuleNode{},
		},
	}
	testGroups2 := []rules.RuleGroup{
		{
			Name:  "baz",
			Rules: []rulefmt.RuleNode{},
		},
	}
	testGroups3 := []rules.RuleGroup{
		{
			Name:  "quux",
			Rules: []rulefmt.RuleNode{},
		},
	}
	It("should wait until any receivers are present before updating rules", func() {

		un := notifier.NewUpdateNotifier(mock_rules.NewTestFinder(ctrl, func() []rules.RuleGroup {
			return notifier.CloneList(testGroups1)
		}))
		// un := rules.NewUpdateNotifier(test.NewTestRuleFinder(ctrl, func() []rules.RuleGroup {
		// 	return notifier.CloneList(testGroups1)
		// }))
		waiting := make(chan struct{})
		go func() {
			defer close(waiting)
			un.Refresh(context.Background())
		}()

		Consistently(waiting).ShouldNot(BeClosed())

		notifiedGroups := <-un.NotifyC(context.Background())
		Expect(notifiedGroups).To(Equal(testGroups1))

		Expect(waiting).To(BeClosed())
	})

	It("should handle notifying multiple receivers", func() {

		un := notifier.NewUpdateNotifier(mock_rules.NewTestFinder(ctrl, func() []rules.RuleGroup {
			return notifier.CloneList(testGroups1)
		}))

		count := 100
		notified := make(chan []rules.RuleGroup, count)
		start := sync.WaitGroup{}
		start.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				c := un.NotifyC(context.Background())
				start.Done()
				notified <- <-c
			}()
		}
		start.Wait()

		un.Refresh(context.Background())
		recv := sync.WaitGroup{}
		recv.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				defer recv.Done()
				select {
				case g := <-notified:
					Expect(g).To(Equal(testGroups1))
				case <-time.After(time.Second):
					Fail("timed out waiting for notifier")
				}
			}()
		}
		recv.Wait()
	})
	It("should handle deleting receivers", func() {

		groups := atomic.Value[[]rules.RuleGroup]{}
		groups.Store(testGroups1)
		updateNotifier := notifier.NewUpdateNotifier(mock_rules.NewTestFinder(ctrl, func() []rules.RuleGroup {
			return notifier.CloneList(groups.Load())
		}))

		count := 100
		type ctxCa struct {
			ctx context.Context
			ca  context.CancelFunc
		}
		contexts := make([]ctxCa, count)
		for i := 0; i < count; i++ {
			ctx, ca := context.WithCancel(context.Background())
			contexts[i] = ctxCa{
				ctx: ctx,
				ca:  ca,
			}
		}

		channels := make([]<-chan []rules.RuleGroup, count)
		for i := 0; i < count; i++ {
			channels[i] = updateNotifier.NotifyC(contexts[i].ctx)
		}

		go updateNotifier.Refresh(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups1)))
		}

		groups.Store(testGroups2) // cancel the channels
		for i := 0; i < count; i++ {
			contexts[i].ca()
		}

		go updateNotifier.Refresh(context.Background())

		for i := 0; i < count; i++ {
			Expect(channels[i]).NotTo(Receive())
		}
	})
	It("should handle rule updates", func() {

		groups := atomic.Value[[]rules.RuleGroup]{}
		groups.Store(testGroups1)
		un := notifier.NewUpdateNotifier(mock_rules.NewTestFinder(ctrl, func() []rules.RuleGroup {
			return notifier.CloneList(groups.Load())
		}))

		count := 100
		channels := make([]<-chan []rules.RuleGroup, count)
		for i := 0; i < count; i++ {
			channels[i] = un.NotifyC(context.Background())
		}

		go un.Refresh(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups1)))
		}

		groups.Store(testGroups2)

		go un.Refresh(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups2)))
		}

		go un.Refresh(context.Background())

		for i := 0; i < count; i++ {
			Expect(channels[i]).NotTo(Receive())
		}

		//Safe convert alias back
		testGroups3_rulefmt := []rulefmt.RuleGroup{}
		for _, group3 := range testGroups3 {
			testGroups3_rulefmt = append(testGroups3_rulefmt, (rulefmt.RuleGroup)(group3))
		}

		groups.Store(testGroups3)

		go un.Refresh(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups3)))
		}
	})
	When("a receiver is not listening on its channel", func() {

		It("should not block the update notifier", func() {
			groups := testGroups1
			un := notifier.NewUpdateNotifier(mock_rules.NewTestFinder(ctrl, func() []rules.RuleGroup {
				return notifier.CloneList(groups)
			}))
			ch := un.NotifyC(context.Background())

			go un.Refresh(context.Background())

			Eventually(ch).Should(Receive(Equal(testGroups1)))

			for i := 0; i < cap(ch)*2; i++ {
				groups = lo.Ternary(i%2 == 0, testGroups2, testGroups3)
				un.Refresh(context.Background())
			}

			for i := 0; i < cap(ch); i++ {
				Expect(ch).To(Receive(Or(Equal(testGroups2), Equal(testGroups3))))
			}
			Consistently(ch).ShouldNot(Receive())
		})
	})
})
