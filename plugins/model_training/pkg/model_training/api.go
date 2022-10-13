package model_training

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	util "github.com/rancher/opni/pkg/util/k8sutil"
	model_training "github.com/rancher/opni/plugins/model_training/pkg/apis/model_training"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
)

func (c *ModelTrainingPlugin) TrainModel(ctx context.Context, in *model_training.WorkloadsList) (*corev1.Reference, error) {
	var model_training_parameters = map[string]map[string][]string{}
	for idx := 0; idx < len(in.List); idx++ {
		cluster_id := in.List[idx].ClusterId
		namespace_name := in.List[idx].Namespace
		deployment_name := in.List[idx].Deployment
		_, cluster_found := model_training_parameters[cluster_id]
		if cluster_found {
			_, namespace_found := model_training_parameters[cluster_id][namespace_name]
			if namespace_found {
				model_training_parameters[cluster_id][namespace_name] = append(model_training_parameters[cluster_id][namespace_name], deployment_name)
			} else {
				model_training_parameters[cluster_id][namespace_name] = make([]string, 0)
				model_training_parameters[cluster_id][namespace_name] = append(model_training_parameters[cluster_id][namespace_name], deployment_name)
			}
		} else {
			model_training_parameters[cluster_id] = map[string][]string{}
			model_training_parameters[cluster_id][namespace_name] = make([]string, 0)
			model_training_parameters[cluster_id][namespace_name] = append(model_training_parameters[cluster_id][namespace_name], deployment_name)
		}
	}
	json_parameters, _ := json.Marshal(model_training_parameters)
	json_bytes := []byte(json_parameters)
	msg, err := c.natsConnection.Get().Request("train_model", json_bytes, time.Minute)
	if err != nil {
		return nil, err
	}
	res := corev1.Reference{Id: string(msg.Data)}
	return &res, nil
}

func (c *ModelTrainingPlugin) WorkloadLogCount(ctx context.Context, in *corev1.Reference) (*model_training.WorkloadsList, error) {
	result, err := c.kv.Get().Get("aggregation")
	if err != nil {
		return nil, err
	}
	jsonRes := result.Value()
	var results_storage = map[string]map[string]map[string]int{}
	if err := json.Unmarshal(jsonRes, &results_storage); err != nil {
		return nil, err
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
	msg, err := c.natsConnection.Get().Request("model_status", b, time.Minute)
	if err != nil {
		return nil, err
	}
	res := corev1.Reference{Id: string(msg.Data)}
	return &res, nil
}

func (c *ModelTrainingPlugin) ModelTrainingParameters(ctx context.Context, in *emptypb.Empty) (*model_training.WorkloadsList, error) {
	b := []byte("model_training_parameters")
	msg, err := c.natsConnection.Get().Request("workload_parameters", b, time.Minute)
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

func (c *ModelTrainingPlugin) GpuPresentCluster(ctx context.Context, in *emptypb.Empty) (*model_training.GPUInfoList, error) {
	client, err := util.NewK8sClient(util.ClientOptions{})
	if err != nil {
		return nil, err
	}
	nodes := &k8scorev1.NodeList{}
	if err := client.List(ctx, nodes); err != nil {
		return nil, err
	}
	returned_data := model_training.GPUInfoList{}
	gpu_info_array := make([]*model_training.GPUInfo, 0)

	for _, node := range nodes.Items {
		//labels := node.Labels
		capacity := node.Status.Capacity
		allocatable := node.Status.Allocatable
		for k, v := range capacity {
			if strings.HasPrefix(string(k), "nvidia.com/gpu") {
				allocation := allocatable[k]
				gpu_info := &model_training.GPUInfo{Name: string(k), Capacity: v.String(), Allocatable: allocation.String()}
				gpu_info_array = append(gpu_info_array, gpu_info)
			}
		}
	}
	returned_data.List = gpu_info_array
	return &returned_data, nil
}
