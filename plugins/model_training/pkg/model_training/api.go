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
	json_aggregation_results, _ := json.Marshal(cluster_aggregation_results)
	workload_response := *&model_training.WorkloadResponse{}
	workload_response.Message = string(json_aggregation_results)
	return &workload_response, nil
}
