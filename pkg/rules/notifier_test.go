package rules_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/samber/lo"

	"github.com/rancher/opni-monitoring/pkg/rules"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
)

var _ = Describe("Update Notifier", func() {
	testGroups1 := []rulefmt.RuleGroup{
		{
			Name:  "foo",
			Rules: []rulefmt.RuleNode{},
		},
		{
			Name:  "bar",
			Rules: []rulefmt.RuleNode{},
		},
	}
	testGroups2 := []rulefmt.RuleGroup{
		{
			Name:  "baz",
			Rules: []rulefmt.RuleNode{},
		},
	}
	testGroups3 := []rulefmt.RuleGroup{
		{
			Name:  "quux",
			Rules: []rulefmt.RuleNode{},
		},
	}
	It("should wait until any receivers are present before updating rules", func() {
		un := rules.NewUpdateNotifier(test.NewTestRuleFinder(ctrl, func() []rulefmt.RuleGroup {
			return rules.CloneRuleGroupList(testGroups1)
		}))
		waiting := make(chan struct{})
		go func() {
			defer close(waiting)
			un.FetchRules(context.Background())
		}()

		Consistently(waiting).ShouldNot(BeClosed())

		notifiedGroups := <-un.NotifyC(context.Background())
		Expect(notifiedGroups).To(Equal(testGroups1))

		Expect(waiting).To(BeClosed())
	})

	It("should handle notifying multiple receivers", func() {
		un := rules.NewUpdateNotifier(test.NewTestRuleFinder(ctrl, func() []rulefmt.RuleGroup {
			return rules.CloneRuleGroupList(testGroups1)
		}))

		count := 100
		notified := make(chan []rulefmt.RuleGroup, count)
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

		un.FetchRules(context.Background())
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
		groups := testGroups1
		un := rules.NewUpdateNotifier(test.NewTestRuleFinder(ctrl, func() []rulefmt.RuleGroup {
			return rules.CloneRuleGroupList(groups)
		}))

		count := 100
		type ctxCa struct {
			ctx context.Context
			ca  context.CancelFunc
		}
		contexts := make([]ctxCa, count)
		for i := 0; i < count; i++ {
			ctx, ca := context.WithCancel(waitctx.Background())
			contexts[i] = ctxCa{
				ctx: ctx,
				ca:  ca,
			}
		}

		channels := make([]<-chan []rulefmt.RuleGroup, count)
		for i := 0; i < count; i++ {
			channels[i] = un.NotifyC(contexts[i].ctx)
		}

		go un.FetchRules(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups1)))
		}

		groups = testGroups2

		for i := 0; i < count; i++ {
			contexts[i].ca()
			waitctx.Wait(contexts[i].ctx, 5*time.Second)
		}

		go un.FetchRules(context.Background())

		for i := 0; i < count; i++ {
			Expect(channels[i]).NotTo(Receive())
		}
	})
	It("should handle rule updates", func() {
		groups := testGroups1
		un := rules.NewUpdateNotifier(test.NewTestRuleFinder(ctrl, func() []rulefmt.RuleGroup {
			return rules.CloneRuleGroupList(groups)
		}))

		count := 100
		channels := make([]<-chan []rulefmt.RuleGroup, count)
		for i := 0; i < count; i++ {
			channels[i] = un.NotifyC(context.Background())
		}

		go un.FetchRules(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups1)))
		}

		groups = testGroups2

		go un.FetchRules(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups2)))
		}

		go un.FetchRules(context.Background())

		for i := 0; i < count; i++ {
			Expect(channels[i]).NotTo(Receive())
		}

		groups = testGroups3

		go un.FetchRules(context.Background())

		for i := 0; i < count; i++ {
			Eventually(channels[i]).Should(Receive(Equal(testGroups3)))
		}
	})
	When("a receiver is not listening on its channel", func() {
		It("should not block the update notifier", func() {
			groups := testGroups1
			un := rules.NewUpdateNotifier(test.NewTestRuleFinder(ctrl, func() []rulefmt.RuleGroup {
				return rules.CloneRuleGroupList(groups)
			}))
			ch := un.NotifyC(context.Background())

			go un.FetchRules(context.Background())

			Eventually(ch).Should(Receive(Equal(testGroups1)))

			for i := 0; i < cap(ch)*2; i++ {
				groups = lo.Ternary(i%2 == 0, testGroups2, testGroups3)
				un.FetchRules(context.Background())
			}

			for i := 0; i < cap(ch); i++ {
				Expect(ch).To(Receive(Or(Equal(testGroups2), Equal(testGroups3))))
			}
			Consistently(ch).ShouldNot(Receive())
		})
	})
})
