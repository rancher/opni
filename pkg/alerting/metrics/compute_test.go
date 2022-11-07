package metrics_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/metrics"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

var _ = Describe("compute alerting options pipeline & compute alerts construction", func() {
	When("users want to create cpu compute alerts", func() {
		Specify("alerting/metrics package should export cpu alerts", func() {
			var err error
			Expect(err).To(BeNil())
		})

		It("should be able to create CPU saturation rules", func() {
			rule, err := metrics.NewCpuRule(
				map[string]*alertingv1.Cores{},
				[]string{"user", "guest", "system"},
				">",
				0.5,
				durationpb.New(time.Minute),
				map[string]string{},
			)
			Expect(err).To(Succeed())
			_, err = rule.Build(uuid.New().String())
			Expect(err).To(Succeed())
		})

		It("should be able to create Memory saturation rules", func() {
			rule, err := metrics.NewMemRule(
				map[string]*alertingv1.MemoryInfo{},
				[]string{"Cached"},
				">",
				90.0,
				durationpb.New(time.Minute),
				map[string]string{},
			)
			Expect(err).To(Succeed())
			_, err = rule.Build(uuid.New().String())
			Expect(err).To(Succeed())
		})

		It("should be able to create Filesystem saturation rules", func() {
			rule, err := metrics.NewFsRule(
				map[string]*alertingv1.FilesystemInfo{},
				">",
				90.0,
				durationpb.New(time.Minute),
				map[string]string{},
			)
			Expect(err).To(Succeed())
			_, err = rule.Build(uuid.New().String())
			Expect(err).To(Succeed())
		})
	})
})
