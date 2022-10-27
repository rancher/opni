package modeltraining

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	modeltraining "github.com/rancher/opni/plugins/modeltraining/pkg/apis/modeltraining"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

func (c *ModelTrainingPlugin) TrainModel(ctx context.Context, in *modeltraining.ModelTrainingParametersList) (*modeltraining.ModelTrainingResponse, error) {
	var modelTrainingParameters = map[string]map[string][]string{}
	for _, item := range in.Items {
		clusterId := item.ClusterId
		namespaceName := item.Namespace
		deploymentName := item.Deployment
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
		return nil, status.Errorf(codes.Internal, "Failed to marshal model training parameters: %v", err)
	}
	msg, err := c.natsConnection.Get().Request("train_model", jsonParameters, time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to train model: %v", err)
	}
	return &modeltraining.ModelTrainingResponse{
		Response: string(msg.Data),
	}, nil
}

func (c *ModelTrainingPlugin) ClusterWorkloadAggregation(ctx context.Context, in *corev1.Reference) (*modeltraining.WorkloadAggregationList, error) {
	result, err := c.kv.Get().Get("aggregation")
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Failed to get workload aggregation from Jetstream: %s", err)
	}
	jsonRes := result.Value()
	var resultsStorage = Aggregations{}
	if err := json.Unmarshal(jsonRes, &resultsStorage); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal workload aggregation from Jetstream: %s", err)
	}
	clusterAggregationResults, ok := resultsStorage.ByCluster[in.Id]
	if !ok {
		return &modeltraining.WorkloadAggregationList{}, nil
	}
	workloadArray := make([]*modeltraining.WorkloadAggregation, 0)
	for namespaceName, deployments := range clusterAggregationResults.ByNamespace {
		for deploymentName, count := range deployments.ByDeployment {
			workloadAggregation := modeltraining.WorkloadAggregation{ClusterId: in.Id, Namespace: namespaceName, Deployment: deploymentName, LogCount: int64(count.Count)}
			workloadArray = append(workloadArray, &workloadAggregation)
		}
	}
	return &modeltraining.WorkloadAggregationList{
		Items: workloadArray,
	}, nil
}

func (c *ModelTrainingPlugin) GetModelStatus(ctx context.Context, in *emptypb.Empty) (*modeltraining.ModelStatus, error) {
	b := []byte("model_status")
	msg, err := c.natsConnection.Get().Request("model_status", b, time.Minute)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to get model status.")
	}
	return &modeltraining.ModelStatus{
		Status: string(msg.Data),
	}, nil
}

func (c *ModelTrainingPlugin) GetModelTrainingParameters(ctx context.Context, in *emptypb.Empty) (*modeltraining.ModelTrainingParametersList, error) {
	b := []byte("model_training_parameters")
	msg, err := c.natsConnection.Get().Request("workload_parameters", b, time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training parameters. %s", err)
	}
	parametersArray := make([]*modeltraining.ModelTrainingParameters, 0)
	var resultsStorage = map[string]map[string][]string{}
	if err := json.Unmarshal(msg.Data, &resultsStorage); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal JSON: %s", err)
	}
	for clusterName, namespaces := range resultsStorage {
		for namespaceName, deployments := range namespaces {
			if deployments == nil {
				c.Logger.Error("Unexpected nil deployment for array.")
				continue
			}
			for deploymentIdx := range deployments {
				deploymentData := modeltraining.ModelTrainingParameters{ClusterId: clusterName, Namespace: namespaceName, Deployment: deployments[deploymentIdx]}
				parametersArray = append(parametersArray, &deploymentData)
			}
		}
	}
	return &modeltraining.ModelTrainingParametersList{
		Items: parametersArray,
	}, nil
}

func (c *ModelTrainingPlugin) ClusterGPUInfo(ctx context.Context, in *emptypb.Empty) (*modeltraining.GPUInfoList, error) {

	nodes := &k8scorev1.NodeList{}
	if err := c.k8sClient.Get().List(ctx, nodes); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "Failed to find nodes: %s", err)
		} else {
			return nil, status.Errorf(codes.Internal, "Failed to get nodes: %v", err)
		}
	}
	gpuInfoArray := make([]*modeltraining.GPUInfo, 0)

	for _, node := range nodes.Items {
		capacity := node.Status.Capacity
		allocatable := node.Status.Allocatable
		for k, v := range capacity {
			if strings.HasPrefix(string(k), "nvidia.com/gpu") {
				allocation := allocatable[k]
				gpuInfo := &modeltraining.GPUInfo{Name: string(k), Capacity: v.String(), Allocatable: allocation.String()}
				gpuInfoArray = append(gpuInfoArray, gpuInfo)
			}
		}
	}
	return &modeltraining.GPUInfoList{
		Items: gpuInfoArray,
	}, nil
}
