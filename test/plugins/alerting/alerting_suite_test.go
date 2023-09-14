package alerting_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	_ "github.com/rancher/opni/pkg/test/setup"
	_ "github.com/rancher/opni/plugins/alerting/test"
	_ "github.com/rancher/opni/plugins/metrics/test"
	"github.com/samber/lo"
)

func init() {
	routing.DefaultConfig = routing.Config{
		GlobalConfig: routing.GlobalConfig{
			GroupWait:      lo.ToPtr(model.Duration(1 * time.Second)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		SubtreeConfig: routing.SubtreeConfig{
			GroupWait:      lo.ToPtr(model.Duration(1 * time.Second)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		FinalizerConfig: routing.FinalizerConfig{
			InitialDelay:       time.Second * 1,
			ThrottlingDuration: time.Minute * 1,
			RepeatInterval:     time.Hour * 5,
		},
		NotificationConfg: routing.NotificationConfg{},
	}
}

func TestAlerting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alerting Suite")
}

// var env *test.Environment
// var tmpConfigDir string

// var _ = BeforeSuite(func() {
// 	testruntime.IfIntegration(func() {
// 		env = &test.Environment{}
// 		Expect(env).NotTo(BeNil())
// 		Expect(env.Start()).To(Succeed())
// 		DeferCleanup(env.Stop)
// 		tmpConfigDir = env.GenerateNewTempDirectory("alertmanager-config")
// 		Expect(tmpConfigDir).NotTo(Equal(""))
// 	})
// })
