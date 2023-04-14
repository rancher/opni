package health_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/mock/auth"
	"github.com/rancher/opni/pkg/test/mock/health"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/health"
)

var _ = Describe("Monitor", func() {
	When("creating a new monitor", func() {
		It("should have no current health status", func() {
			monitor := health.NewMonitor()
			healthStatus := monitor.GetHealthStatus("nonexistent")
			Expect(healthStatus).NotTo(BeNil())
			Expect(healthStatus.Health).To(BeNil())
			Expect(healthStatus.Status).To(BeNil())
		})
	})
	It("should listen to a health status updater", func() {
		agent1 := &mock_health.HealthStore{}
		agent1.SetHealth(&corev1.Health{Ready: false, Conditions: []string{"foo"}})
		listener := newFastListener()
		defer listener.Close()

		agentCtx, agentCa := context.WithCancel(context.Background())
		monitorCtx, monitorCa := context.WithCancel(context.Background())
		defer monitorCa()
		go listener.HandleConnection(mock_auth.ContextWithAuthorizedID(agentCtx, "agent1"), agent1)
		monitor := health.NewMonitor()
		go monitor.Run(monitorCtx, listener)

		Eventually(func() error {
			hs := monitor.GetHealthStatus("agent1")
			if len(hs.GetHealth().GetConditions()) != 1 {
				return errors.New("wrong number of conditions")
			}
			if hs.GetHealth().GetReady() {
				return errors.New("should not be ready")
			}
			if !hs.GetStatus().GetConnected() {
				return errors.New("not connected")
			}
			return nil
		}).Should(Succeed())

		agent1.SetHealth(&corev1.Health{Ready: true})

		Eventually(func() error {
			hs := monitor.GetHealthStatus("agent1")
			if !hs.GetHealth().GetReady() {
				return errors.New("not ready")
			}
			if !hs.GetStatus().GetConnected() {
				return errors.New("not connected")
			}
			return nil
		}).Should(Succeed())

		agentCa()

		Eventually(func() error {
			hs := monitor.GetHealthStatus("agent1")
			if hs.GetHealth().GetReady() {
				return errors.New("still ready")
			}
			if hs.GetStatus().GetConnected() {
				return errors.New("still connected")
			}
			return nil
		}).Should(Succeed())
	})
	It("should stop if the updater's health or status channels are closed", func() {
		monitor := health.NewMonitor()
		listener := health.NewListener()
		stopped := make(chan struct{})
		go func() {
			monitor.Run(context.Background(), listener)
			close(stopped)
		}()

		Consistently(stopped).ShouldNot(BeClosed())
		listener.Close()
		Eventually(stopped).Should(BeClosed())
	})
})
