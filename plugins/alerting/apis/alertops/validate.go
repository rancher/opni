package alertops

import (
	prommodel "github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/validation"
	"k8s.io/apimachinery/pkg/api/resource"
)

func (c *ClusterConfiguration) Validate() error {
	if c == nil {
		return validation.Error("ClusterConfiguration must be set")
	}
	if c.NumReplicas < 1 {
		return validation.Error("Number of opni alerting replicas must at least be 1")
	}
	if c.ResourceLimits == nil {
		return validation.Error("ResourceSpec must be set")
	}
	if c.ClusterGossipInterval == "" {
		return validation.Error("ClusterGossipInterval must be set")
	}
	if c.ClusterPushPullInterval == "" {
		return validation.Error("ClusterPushPullInterval must be set")
	}
	if c.ClusterSettleTimeout == "" {
		return validation.Error("ClusterSettleTimeout must be set")
	}
	if _, err := prommodel.ParseDuration(c.ClusterGossipInterval); err != nil {
		return validation.Errorf("invalid gossip interval : %v", err)
	}
	if _, err := prommodel.ParseDuration(c.ClusterPushPullInterval); err != nil {
		return validation.Errorf("invalid push pull interval : %v", err)
	}
	if _, err := prommodel.ParseDuration(c.ClusterSettleTimeout); err != nil {
		return validation.Errorf("invalid settle timeout : %v", err)
	}

	return c.ResourceLimits.Validate()
}

func (r *ResourceLimitSpec) Validate() error {
	if r.Cpu == "" {
		return validation.Error("CPU must be set")
	}
	if r.Memory == "" {
		return validation.Error("Memory must be set")
	}
	if r.Storage == "" {
		return validation.Error("Storage must be set")
	}

	if _, cpuErr := resource.ParseQuantity(r.Cpu); cpuErr != nil {
		return validation.Errorf("invalid cpu resource quantity : %v", cpuErr)
	}
	if _, memErr := resource.ParseQuantity(r.Memory); memErr != nil {
		return validation.Errorf("invalid memory resource quantity : %v", memErr)
	}

	if _, storageErr := resource.ParseQuantity(r.Storage); storageErr != nil {
		return validation.Errorf("invalid storage resource quantity : %v", storageErr)
	}
	return nil
}
