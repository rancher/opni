package model_training

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	util "github.com/rancher/opni/pkg/util/k8sutil"
	modelTraining "github.com/rancher/opni/plugins/modelTraining/pkg/apis/modelTraining"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
)

func (c *ModelTrainingPlugin) TrainModel(ctx context.Context, in *modelTraining.WorkloadsList) (*corev1.Reference, error) {
	var modelTrainingParameters = map[string]map[string][]string{}
	for idx := 0; idx < len(in.List); idx++ {
		clusterId := in.List[idx].ClusterId
		namespaceName := in.List[idx].Namespace
		deploymentName := in.List[idx].Deployment
		_, clusterFound := modelTrainingParameters[clusterId]
		if clusterFound {
			_, namespaceFound := modelTrainingParameters[clusterId][namespaceName]
			if namespaceFound {
				modelTrainingParameters[clusterId][namespaceName] = append(modelTrainingParameters[clusterId][namespaceName], deploymentName)
			} else {
				modelTrainingParameters[clusterId][namespaceName] = make([]string, 0)
				modelTrainingParameters[clusterId][namespaceName] = append(modelTrainingParameters[clusterId][namespaceName], deploymentName)
			}
		} else {
			modelTrainingParameters[clusterId] = map[string][]string{}
			modelTrainingParameters[clusterId][namespaceName] = make([]string, 0)
			modelTrainingParameters[clusterId][namespaceName] = append(modelTrainingParameters[clusterId][namespaceName], deploymentName)
		}
	}
	jsonParameters, err := json.Marshal(modelTrainingParameters)
	if err != nil {
		return nil, err
	}
	jsonBytes := []byte(jsonParameters)
	msg, err := c.natsConnection.Get().Request("train_model", jsonBytes, time.Minute)
	if err != nil {
		return nil, err
	}
	res := corev1.Reference{Id: string(msg.Data)}
	return &res, nil
}

func (c *ModelTrainingPlugin) WorkloadLogCount(ctx context.Context, in *corev1.Reference) (*modelTraining.WorkloadsList, error) {
	result, err := c.kv.Get().Get("aggregation")
	if err != nil {
		return nil, err
	}
	jsonRes := result.Value()
	var resultsStorage = Aggregations{}
	if err := json.Unmarshal(jsonRes, &resultsStorage); err != nil {
		return nil, err
	}
	clusterAggregationResults, ok := resultsStorage.ByCluster[in.Id]
	if !ok {
		return nil, nil
	}
	workloadsList := modelTraining.WorkloadsList{}
	workloadArray := make([]*modelTraining.WorkloadResponse, 0)
	for namespaceName, deployments := range clusterAggregationResults.ByNamespace {
		for deploymentName, count := range deployments.ByDeployment {
			workload_aggregation := modelTraining.WorkloadResponse{ClusterId: in.Id, Namespace: namespaceName, Deployment: deploymentName, Count: int64(count.Count)}
			workloadArray = append(workloadArray, &workload_aggregation)
		}
	}
	workloadsList.List = workloadArray
	return &workloadsList, nil
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

func (c *ModelTrainingPlugin) ModelTrainingParameters(ctx context.Context, in *emptypb.Empty) (*modelTraining.WorkloadsList, error) {
	b := []byte("model_training_parameters")
	msg, err := c.natsConnection.Get().Request("workload_parameters", b, time.Minute)
	if err != nil {
		return nil, err
	}
	trainingParameters := modelTraining.WorkloadsList{}
	parametersArray := make([]*modelTraining.WorkloadResponse, 0)
	var results_storage = map[string]map[string][]string{}
	if err := json.Unmarshal(msg.Data, &results_storage); err != nil {
		return nil, err
	}
	for clusterName, namespaces := range results_storage {
		for namespaceName, deployments := range namespaces {
			if deployments == nil {
				c.Logger.Error("Unexpected nil deployment for array.")
				continue
			}
			for deployment_idx := range deployments {
				deployment_data := modelTraining.WorkloadResponse{ClusterId: clusterName, Namespace: namespaceName, Deployment: deployments[deployment_idx]}
				parametersArray = append(parametersArray, &deployment_data)
			}
		}
	}
	trainingParameters.List = parametersArray
	return &trainingParameters, nil
}

func (c *ModelTrainingPlugin) GpuPresentCluster(ctx context.Context, in *emptypb.Empty) (*modelTraining.GPUInfoList, error) {
	client, err := util.NewK8sClient(util.ClientOptions{})
	if err != nil {
		return nil, err
	}
	nodes := &k8scorev1.NodeList{}
	if err := client.List(ctx, nodes); err != nil {
		return nil, err
	}
	returnedData := modelTraining.GPUInfoList{}
	gpuInfoArray := make([]*modelTraining.GPUInfo, 0)

	for _, node := range nodes.Items {
		capacity := node.Status.Capacity
		allocatable := node.Status.Allocatable
		for k, v := range capacity {
			if strings.HasPrefix(string(k), "nvidia.com/gpu") {
				allocation := allocatable[k]
				gpuInfo := &modelTraining.GPUInfo{Name: string(k), Capacity: v.String(), Allocatable: allocation.String()}
				gpuInfoArray = append(gpuInfoArray, gpuInfo)
			}
		}
	}
	returnedData.List = gpuInfoArray
	return &returnedData, nil
}
