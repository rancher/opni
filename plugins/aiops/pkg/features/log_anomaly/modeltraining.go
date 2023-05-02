package log_anomaly

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/admin"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	modelTrainingParametersKey = "modelTrainingParameters"
	modelTrainingStatusKey     = "modelTrainingStatus"
	logAggregationCountKey     = "aggregation"
	modelTrainingNatsSubject   = "train_model"
	modelStatusNatsSubject     = "model_status"
)

type ModelTrainingParameters struct {
	UUID      string                         `json:"uuid,omitempty"`
	Workloads map[string]map[string][]string `json:"workloads,omitempty"`
}

func (p *LogAnomaly) TrainModel(ctx context.Context, in *modeltraining.ModelTrainingParametersList) (*modeltraining.ModelTrainingResponse, error) {
	var modelTrainingParameters = map[string]map[string][]string{}
	p.LaunchAIServices(ctx)
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
	uuidGenerated := uuid.New().String()
	modelRes := ModelTrainingParameters{UUID: uuidGenerated, Workloads: modelTrainingParameters}
	jsonParameters, err := json.Marshal(modelRes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal model training parameters: %v", err)
	}
	parametersBytes := []byte(jsonParameters)
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	modelTrainingKv, err := p.modelTrainingKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training parameters: %v", err)
	}
	modelTrainingKv.Put(modelTrainingParametersKey, parametersBytes)
	if len(modelTrainingParameters) > 0 {
		initialStatus := modeltraining.ModelStatus{Status: "training", Statistics: &modeltraining.ModelTrainingStatistics{Stage: "fetching data"}}
		_, err = p.PutModelTrainingStatus(ctx, &initialStatus)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to put model training status: %v", err)
		}
	} else {
		initialStatus := modeltraining.ModelStatus{Status: "no model trained"}
		_, err = p.PutModelTrainingStatus(ctx, &initialStatus)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to put model training status: %v", err)
		}
	}
	natsConnection, err := p.natsConnection.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training parameters: %v", err)
	}
	msg, err := natsConnection.Request(modelTrainingNatsSubject, jsonParameters, time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "Failed to train model: %v", err)
	}
	return &modeltraining.ModelTrainingResponse{
		Response: string(msg.Data),
	}, nil
}

func (p *LogAnomaly) LaunchAIServices(ctx context.Context) (*emptypb.Empty, error) {
	aiSettings, err := p.GetAISettings(ctx, &emptypb.Empty{})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			aiSettings = &admin.AISettings{GpuSettings: &admin.GPUSettings{}}
		} else {
			return nil, status.Errorf(codes.Internal, "Failed to get AI settings: %v", err)
		}
	}
	if aiSettings.GpuSettings == nil {
		aiSettings.GpuSettings = &admin.GPUSettings{}
	}
	p.PutAISettings(ctx, aiSettings)
	return nil, nil
}

func (p *LogAnomaly) PutModelTrainingStatus(ctx context.Context, in *modeltraining.ModelStatus) (*emptypb.Empty, error) {
	jsonParameters, err := protojson.Marshal(in)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshal model training statistics: %v", err)
	}
	bytesAggregation := []byte(jsonParameters)
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	statisticsKv, err := p.statisticsKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training statistics: %v", err)
	}
	statisticsKv.Put(modelTrainingStatusKey, bytesAggregation)
	return nil, nil
}

func (p *LogAnomaly) ClusterWorkloadAggregation(ctx context.Context, in *corev1.Reference) (*modeltraining.WorkloadAggregationList, error) {
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	aggregationKv, err := p.aggregationKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get workload aggregation from Jetstream: %v", err)
	}
	result, err := aggregationKv.Get(logAggregationCountKey)
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

func (p *LogAnomaly) GetModelStatus(ctx context.Context, _ *emptypb.Empty) (*modeltraining.ModelStatus, error) {
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	statisticsKv, err := p.statisticsKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training status from Jetstream: %v", err)
	}
	result, err := statisticsKv.Get(modelTrainingStatusKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return &modeltraining.ModelStatus{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to get model training status from Jetstream: %v", err)
	}
	jsonRes := result.Value()
	var resultsStorage = modeltraining.ModelStatus{}
	if err := protojson.Unmarshal(jsonRes, &resultsStorage); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal model training status from Jetstream: %v", err)
	}
	return &resultsStorage, nil
}

func (p *LogAnomaly) GetModelTrainingParameters(ctx context.Context, _ *emptypb.Empty) (*modeltraining.ModelTrainingParametersList, error) {
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	modelTrainingKv, err := p.modelTrainingKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get context for model training parameters from Jetstream: %v", err)
	}
	result, err := modelTrainingKv.Get(modelTrainingParametersKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return &modeltraining.ModelTrainingParametersList{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "Failed to get model training parameters from Jetstream: %v", err)
	}
	var parametersArray []*modeltraining.ModelTrainingParameters
	var kvContent ModelTrainingParameters
	jsonRes := result.Value()
	if err := json.Unmarshal(jsonRes, &kvContent); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmarshal model training parameters from Jetstream: %v", err)
	}
	resultsStorage := kvContent.Workloads
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

func (p *LogAnomaly) GPUInfo(ctx context.Context, _ *emptypb.Empty) (*modeltraining.GPUInfoList, error) {
	nodes := &k8scorev1.NodeList{}
	err := p.K8sClient.List(ctx, nodes)
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
