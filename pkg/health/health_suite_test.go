package health_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/health"
)

func TestHealth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Health Suite")
}

func newFastListener(opts ...health.ListenerOption) *health.Listener {
	return health.NewListener(append(opts,
		health.WithPollInterval(25*time.Millisecond),
		health.WithMaxJitter(0),
	)...)
}

func checkHealth(listener *health.Listener, id string, ready bool, conditions ...string) {
	select {
	case update := <-listener.HealthC():
		ExpectWithOffset(1, update.ID).To(Equal(id))
		ExpectWithOffset(1, update.Health.Ready).To(Equal(ready))
		ExpectWithOffset(1, update.Health.Conditions).To(HaveLen(len(conditions)))
		ExpectWithOffset(1, update.Health.Conditions).To(ContainElements(conditions))
	case <-time.After(1 * time.Second):
		Fail("timeout waiting for health update", 1)
	}
}

func checkStatus(listener *health.Listener, id string, connected bool) {
	select {
	case update := <-listener.StatusC():
		ExpectWithOffset(1, update.ID).To(Equal(id))
		ExpectWithOffset(1, update.Status.Connected).To(Equal(connected))
	case <-time.After(1 * time.Second):
		Fail("timeout waiting for status update", 1)
	}
}
