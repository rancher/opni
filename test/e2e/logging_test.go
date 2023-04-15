//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/gateway"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Logging Test", Ordered, Label("e2e", "slow"), func() {
	var ctx context.Context
	BeforeAll(func() {
		ctx = context.Background()

		_, err := loggingAdminClient.CreateOrUpdateOpensearchCluster(ctx, &loggingadmin.OpensearchCluster{
			ExternalURL: outputs.OpensearchURL,
			Dashboards: &loggingadmin.DashboardsDetails{
				Enabled:  lo.ToPtr(true),
				Replicas: lo.ToPtr(int32(1)),
			},
			NodePools: []*loggingadmin.OpensearchNodeDetails{
				{
					Name: "test",
					Roles: []string{
						"controlplane",
						"data",
						"ingest",
					},
					DiskSize:    "50Gi",
					MemoryLimit: "2Gi",
					Persistence: &loggingadmin.DataPersistence{
						Enabled: lo.ToPtr(true),
					},
					Replicas:           lo.ToPtr(int32(3)),
					EnableAntiAffinity: lo.ToPtr(true),
				},
			},
			DataRetention: lo.ToPtr("7d"),
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() error {
			installStatus, err := loggingAdminClient.GetOpensearchStatus(ctx, &emptypb.Empty{})
			if err != nil {
				return err
			}
			if installStatus.Status != int32(gateway.ClusterStatusGreen) {
				return fmt.Errorf("waiting for opensearch cluster to be green")
			}
			return nil
		}, 20*time.Minute, 10*time.Second).Should(Succeed())
	})
	It("should do", func() {
		Expect(true).To(BeTrue())
	})
})
