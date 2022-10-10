package model_training

import (
	"context"
	"encoding/json"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	model_training "github.com/rancher/opni/plugins/model_training/pkg/apis/model_training"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (c *ModelTrainingPlugin) TrainModel(ctx context.Context, in *model_training.WorkloadsList) (*corev1.Reference, error) {
	var model_training_parameters = map[string]map[string][]string{};
	for idx := 0; idx < len(in.List); idx++ {
		cluster_id := in.List[idx].ClusterId
		namespace_name := in.List[idx].Namespace
		deployment_name := in.List[idx].Deployment
	}
}

func (c *ModelTrainingPlugin) WorkloadLogCount(ctx context.Context, in *corev1.Reference) (*model_training.WorkloadsList, error) {
	result, _ := c.kv.Get().Get("aggregation")
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

func (c *ModelTrainingPlugin) ModelStatus(ctx context.Context, in *emptypb.Empty) (*corev1.Reference, error) {
	b := []byte("model_status")
	msg, err := c.natsConnection.Get().Request("model_status", b, time.Second)
	if err != nil {
		return nil, err
	}
	res := corev1.Reference{Id: string(msg.Data)}
	return &res, nil
}

func (c *ModelTrainingPlugin) ModelTrainingParameters(ctx context.Context, in *emptypb.Empty) (*model_training.WorkloadsList, error) {
	b := []byte("model_training_parameters")
	msg, err := c.natsConnection.Get().Request("workload_parameters", b, time.Second)
	if err != nil {
		return nil, err
	}
	training_parameters := model_training.WorkloadsList{}
	parameters_array := make([]*model_training.WorkloadResponse, 0)
	var results_storage = map[string]map[string][]string{}
	if err := json.Unmarshal(msg.Data, &results_storage); err != nil {
		return nil, err
	}
	for cluster_name, namespaces := range results_storage {
		for namespace_name, deployments := range namespaces {
			if deployments == nil {
				c.Logger.Error("Unexpected nil deployment for array.")
				continue
			}
			for deployment_idx := range deployments {
				deployment_data := model_training.WorkloadResponse{ClusterId: cluster_name, Namespace: namespace_name, Deployment: deployments[deployment_idx]}
				parameters_array = append(parameters_array, &deployment_data)
			}
		}
	}
	training_parameters.List = parameters_array
	return &training_parameters, nil
}
