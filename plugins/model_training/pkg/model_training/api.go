package model_training

import (
	"context"
	"encoding/json"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	model_training "github.com/rancher/opni/plugins/model_training/pkg/apis/model_training"
)

func (c *ModelTrainingPlugin) WorkloadLogCount(ctx context.Context, in *corev1.Reference) (*model_training.WorkloadsList, error) {
	result, _ := c.kv.Get("aggregation")
	jsonRes := result.Value()
	var results_storage = map[string]map[string]map[string]int{}
	if err := json.Unmarshal(jsonRes, &results_storage); err != nil {
		panic(err)
	}
	cluster_aggregation_results := results_storage[in.Id]
	workloads_list := model_training.WorkloadsList{}
	workload_array := make([]*model_training.WorkloadResponse, 0)
	for namespace_name, deployments := range cluster_aggregation_results {
		for deployment_name, count := range deployments {
			workload_aggregation := model_training.WorkloadResponse{ClusterId: in.Id, Namespace: namespace_name, Deployment: deployment_name, Count: int64(count)}
			workload_array = append(workload_array, &workload_aggregation)
		}
	}
	workloads_list.List = workload_array
	return &workloads_list, nil
}
