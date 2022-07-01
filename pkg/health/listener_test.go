package health_test

import (
	"context"
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Listener", func() {
	When("creating a new listener", func() {
		It("should not send any updates", func() {
			listener := health.NewListener()
			Expect(listener.StatusC()).NotTo(Receive())
			Expect(listener.HealthC()).NotTo(Receive())
		})
	})
	When("handling a new connection", func() {
		It("should send health and status updates", func() {
			agent1 := &test.HealthStore{}
			agent1.SetHealth(&corev1.Health{
				Ready: true,
			})
			listener := health.NewListener()
			go listener.HandleConnection(test.ContextWithAuthorizedID(context.Background(), "agent1"), agent1)

			select {
			case update := <-listener.HealthC():
				Expect(update.ID).To(Equal("agent1"))
				Expect(update.Health.Ready).To(BeTrue())
			case <-time.After(time.Second):
				Fail("timeout waiting for health update")
			}
			select {
			case update := <-listener.StatusC():
				Expect(update.ID).To(Equal("agent1"))
				Expect(update.Status.Connected).To(BeTrue())
			case <-time.After(time.Second):
				Fail("timeout waiting for status update")
			}
		})
	})
	It("should poll for client health updates", func() {
		agent1 := &test.HealthStore{}
		agent1.SetHealth(&corev1.Health{Ready: false})
		id := "agent1"
		listener := newFastListener()
		go listener.HandleConnection(test.ContextWithAuthorizedID(context.Background(), id), agent1)

		checkHealth(listener, id, false)
		checkStatus(listener, id, true)
		for i := 0; i < 10; i++ {
			agent1.SetHealth(&corev1.Health{Ready: true})
			checkHealth(listener, id, true)

			agent1.SetHealth(&corev1.Health{Ready: false, Conditions: []string{"foo"}})
			checkHealth(listener, id, false, "foo")

			agent1.SetHealth(&corev1.Health{Ready: false, Conditions: []string{"bar", "baz"}})
			checkHealth(listener, id, false, "bar", "baz")

			agent1.SetHealth(&corev1.Health{Ready: false})
			checkHealth(listener, id, false)
		}
	})

	It("should send health and status updates when clients disconnect", func() {
		agent1 := &test.HealthStore{}
		agent1.SetHealth(&corev1.Health{Ready: false})
		id := "agent1"
		listener := newFastListener()
		ctx, ca := context.WithCancel(context.Background())
		ctx = test.ContextWithAuthorizedID(ctx, id)
		go listener.HandleConnection(ctx, agent1)

		checkStatus(listener, id, true)
		checkHealth(listener, id, false)
		agent1.SetHealth(&corev1.Health{Ready: true})
		checkHealth(listener, id, true)
		ca()
		checkHealth(listener, id, false)
		checkStatus(listener, id, false)
	})

	It("should preserve status update order on client reconnect", func() {
		numAgents := 100
		listener := health.NewListener(
			health.WithUpdateQueueCap(numAgents*100),
			health.WithMaxConnections(math.MaxInt64),
		)
		statusC := listener.StatusC()
		a := &test.HealthStore{GetHealthShouldFail: true}
		for i := 0; i < numAgents; i++ {

			agentId := fmt.Sprintf("agent%d", i)
			// 49 connects and disconnects = 98 queued status updates for this agent
			for j := 0; j < 49; j++ {
				ctx, ca := context.WithCancel(context.Background())
				ctx = test.ContextWithAuthorizedID(ctx, agentId)
				go listener.HandleConnection(ctx, a)
				ca()
			}
		}

		Eventually(statusC).Should(HaveLen(98 * numAgents))
		Consistently(statusC).Should(HaveLen(98 * numAgents))

		// reconnect all agents
		// need to wait until all the previous reconnects are done, since the mutex
		// held by active connections is not FCFS - this connection could prevent
		// some of the previous reconnects from being processed, breaking the test
		for i := 0; i < numAgents; i++ {
			agentId := fmt.Sprintf("agent%d", i)
			go listener.HandleConnection(test.ContextWithAuthorizedID(context.Background(), agentId), a)
		}

		Eventually(statusC).Should(HaveLen(99 * numAgents))
		Consistently(statusC).Should(HaveLen(99 * numAgents))

		listener.Close()

		Eventually(statusC).Should(HaveLen(100 * numAgents))
		Consistently(statusC).Should(HaveLen(100 * numAgents))

		// for a given agent, a connection update received on the channel should be
		// strictly opposite from the sortedUpdates update received for that agent.
		sortedUpdates := map[string][]*corev1.Status{}
		for update := range statusC {
			sortedUpdates[update.ID] = append(sortedUpdates[update.ID], update.Status)
		}
		Expect(sortedUpdates).To(HaveLen(numAgents))
		for _, updates := range sortedUpdates {
			Expect(len(updates)).To(Equal(100))
			for i := 1; i < len(updates); i++ {
				Expect(updates[i].Connected).To(Equal(!updates[i-1].Connected))
			}
		}

		for _, stat := range sortedUpdates {
			Expect(stat[len(stat)-1].Connected).To(BeFalse())
		}
	})
	It("should not handle additional connections after close", func() {
		listener := health.NewListener()
		listener.Close()
		returned := make(chan struct{})
		go func() {
			defer close(returned)
			listener.HandleConnection(context.Background(), &test.HealthStore{})
		}()
		Eventually(returned).Should(BeClosed())
	})
	It("should enforce a max connection limit", func() {
		listener := health.NewListener(health.WithMaxConnections(1))

		ctx1, ca := context.WithCancel(context.Background())
		ctx1 = test.ContextWithAuthorizedID(ctx1, "agent1")

		ctx2, ca2 := context.WithCancel(context.Background())
		ctx2 = test.ContextWithAuthorizedID(ctx2, "agent2")

		go listener.HandleConnection(ctx1, &test.HealthStore{})

		sc := listener.StatusC()
		stat := <-sc
		Expect(stat.ID).To(Equal("agent1"))
		Expect(stat.Status.Connected).To(BeTrue())

		go listener.HandleConnection(ctx2, &test.HealthStore{})

		Consistently(sc, 1*time.Second, 100*time.Millisecond).ShouldNot(Receive())

		ca()

		receivedAgent1Disconnect := false
		receivedAgent2Connect := false
		for i := 0; i < 2; i++ {
			stat := <-sc
			if stat.ID == "agent1" {
				receivedAgent1Disconnect = true
				Expect(stat.Status.Connected).To(BeFalse())
			} else if stat.ID == "agent2" {
				receivedAgent2Connect = true
				Expect(stat.Status.Connected).To(BeTrue())
			}
		}
		Expect(receivedAgent1Disconnect).To(BeTrue())
		Expect(receivedAgent2Connect).To(BeTrue())

		ctx3, ca3 := context.WithCancel(context.Background())
		ctx3 = test.ContextWithAuthorizedID(ctx3, "agent3")
		go listener.HandleConnection(ctx3, &test.HealthStore{})
		Consistently(sc, 1*time.Second, 100*time.Millisecond).ShouldNot(Receive())
		ca3()
		Consistently(sc, 1*time.Second, 100*time.Millisecond).ShouldNot(Receive())

		ca2()

		stat = <-sc
		Expect(stat.ID).To(Equal("agent2"))
		Expect(stat.Status.Connected).To(BeFalse())
	})
})
