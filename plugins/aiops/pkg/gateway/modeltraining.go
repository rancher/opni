package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	modeltraining "github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

func (p *AIOpsPlugin) TrainModel(ctx context.Context, in *modeltraining.ModelTrainingParametersList) (*modeltraining.ModelTrainingResponse, error) {
	var modelTrainingParameters = map[string]map[string][]string{}
	for _, item := range in.Items {
		clusterId := item.ClusterId
		namespaceName := item.Namespace
		deploymentName := item.Deployment
		_, clusterFound := modelTrainingParameters[clusterId]
		if !clusterFound {
			modelTrainingParameters[clusterId] = map[string][]string{}
		}
		modelTrainingParameters[clusterId][namespaceName] = append(modelTrainingParameters[clusterId][namespaceName], deploymentName)
	}
	jsonParameters, err := json.Marshal(modelTrainingParameters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal model training parameters: %v", err)
	}
	msg, err := p.natsConnection.Get().Request("train_model", jsonParameters, time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "Failed to train model: %v", err)
	}
	_, err = p.PutModelTrainingStatus(ctx, &modeltraining.ModelTrainingStatistics{RemainingTime: 3600})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to put model training status: %v", err)
	}
	return &modeltraining.ModelTrainingResponse{
		Response: string(msg.Data),
	}, nil
}

func (p *AIOpsPlugin) PutModelTrainingStatus(_ context.Context, in *modeltraining.ModelTrainingStatistics) (*emptypb.Empty, error) {
	jsonParameters, err := protojson.Marshal(in)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal model training statistics: %v", err)
	}
	bytesAggregation := []byte(jsonParameters)
	p.kv.Get().Put("modelTrainingStatus", bytesAggregation)
	return nil, nil

}

func (p *AIOpsPlugin) ClusterWorkloadAggregation(_ context.Context, in *corev1.Reference) (*modeltraining.WorkloadAggregationList, error) {
	result, err := p.kv.Get().Get("aggregation")
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Failed to get workload aggregation from Jetstream: %v", err)
	}
	jsonRes := result.Value()
	var resultsStorage = Aggregations{}
	if err := json.Unmarshal(jsonRes, &resultsStorage); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal workload aggregation from Jetstream: %v", err)
	}
	clusterAggregationResults, ok := resultsStorage.ByCluster[in.Id]
	if !ok {
		return &modeltraining.WorkloadAggregationList{}, nil
	}
	var workloadArray []*modeltraining.WorkloadAggregation
	for namespaceName, deployments := range clusterAggregationResults.ByNamespace {
		for deploymentName, count := range deployments.ByDeployment {
			workloadAggregation := modeltraining.WorkloadAggregation{
				ClusterId:  in.Id,
				Namespace:  namespaceName,
				Deployment: deploymentName,
				LogCount:   int64(count.Count),
			}
			workloadArray = append(workloadArray, &workloadAggregation)
		}
	}
	return &modeltraining.WorkloadAggregationList{
		Items: workloadArray,
	}, nil
}

func (p *AIOpsPlugin) GetModelStatus(_ context.Context, _ *emptypb.Empty) (*modeltraining.ModelStatus, error) {
	b := []byte("model_status")
	msg, err := p.natsConnection.Get().Request("model_status", b, time.Minute)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to get model status.")
	}
	result, err := p.kv.Get().Get("modelTrainingStatus")
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return &modeltraining.ModelStatus{
				Status: string(msg.Data),
			}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to get model training status from Jetstream: %v", err)
	}
	jsonRes := result.Value()
	var resultsStorage = modeltraining.ModelTrainingStatistics{}
	if err := protojson.Unmarshal(jsonRes, &resultsStorage); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal model training status from Jetstream: %v", err)
	}
	return &modeltraining.ModelStatus{
		Status:     string(msg.Data),
		Statistics: &resultsStorage,
	}, nil
}

func (p *AIOpsPlugin) GetModelTrainingParameters(_ context.Context, _ *emptypb.Empty) (*modeltraining.ModelTrainingParametersList, error) {
	b := []byte("model_training_parameters")
	msg, err := p.natsConnection.Get().Request("workload_parameters", b, time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training parameters. %v", err)
	}
	var parametersArray []*modeltraining.ModelTrainingParameters
	var resultsStorage = map[string]map[string][]string{}
	if err := json.Unmarshal(msg.Data, &resultsStorage); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal JSON: %v", err)
	}
	for clusterName, namespaces := range resultsStorage {
		for namespaceName, deployments := range namespaces {
			for deploymentIdx := range deployments {
				deploymentData := modeltraining.ModelTrainingParameters{
					ClusterId:  clusterName,
					Namespace:  namespaceName,
					Deployment: deployments[deploymentIdx],
				}
				parametersArray = append(parametersArray, &deploymentData)
			}
		}
	}
	return &modeltraining.ModelTrainingParametersList{
		Items: parametersArray,
	}, nil
}

func (p *AIOpsPlugin) GPUInfo(ctx context.Context, _ *emptypb.Empty) (*modeltraining.GPUInfoList, error) {
	nodes := &k8scorev1.NodeList{}
	err := p.k8sClient.List(ctx, nodes)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "Failed to find nodes: %s", err)
		}
		return nil, status.Errorf(codes.Internal, "Failed to get nodes: %v", err)
	}
	var gpuInfoArray []*modeltraining.GPUInfo

	for _, node := range nodes.Items {
		capacity := node.Status.Capacity
		allocatable := node.Status.Allocatable
		for k, v := range capacity {
			if strings.HasPrefix(string(k), "nvidia.com/gpu") {
				allocation := allocatable[k]
				gpuInfoArray = append(gpuInfoArray, &modeltraining.GPUInfo{
					Name:        string(k),
					Capacity:    v.String(),
					Allocatable: allocation.String(),
				})
			}
		}
	}
	return &modeltraining.GPUInfoList{
		Items: gpuInfoArray,
	}, nil
}
