package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/aiops/apis/admin"
	"github.com/rancher/opni/plugins/aiops/apis/modeltraining"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	modelTrainingParametersKey = "modelTrainingParameters"
	statisticsModelTrainingKey = "modelTrainingStatus"
	logAggregationCountKey     = "aggregation"

	// nats subjects
	modelTrainingNatsSubject = "train_model"
)

type ModelTrainingParameters struct {
	UUID      string                         `json:"uuid,omitempty"`
	Workloads map[string]map[string][]string `json:"workloads,omitempty"`
}

func modelTrainingParams(
	in *modeltraining.ModelTrainingParametersList,
) (params map[string]map[string][]string, bytes []byte, err error) {
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

	uuidGenerated := uuid.New().String()
	modelRes := ModelTrainingParameters{UUID: uuidGenerated, Workloads: modelTrainingParameters}
	jsonParameters, err := json.Marshal(modelRes)
	if err != nil {
		return modelTrainingParameters, nil, status.Errorf(codes.Internal, "Failed to marshal model training parameters: %v", err)
	}
	return modelTrainingParameters, jsonParameters, nil
}

func (p *AIOpsPlugin) requestModelTraining(
	ctx context.Context,
	parametersPayload []byte,
) (*modeltraining.ModelTrainingResponse, error) {
	natsConnection, err := p.natsConnection.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training parameters: %v", err)
	}
	msg, err := natsConnection.Request(modelTrainingNatsSubject, parametersPayload, time.Minute)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "Failed to train model: %v", err)
	}
	return &modeltraining.ModelTrainingResponse{
		Response: string(msg.Data),
	}, nil
}

func (p *AIOpsPlugin) TrainModel(ctx context.Context, in *modeltraining.ModelTrainingParametersList) (*modeltraining.ModelTrainingResponse, error) {
	_, err := p.LaunchAIServices(ctx)
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("failed to launch AI services : %s", err))
	}
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	modelTrainingKv, err := p.modelTrainingKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "Failed to get model training KV: %v", err)
	}

	modelTrainingParameters, parametersBytes, err := modelTrainingParams(in)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to marshall training parameters: %v", err)
	}

	if _, err := modelTrainingKv.Put(modelTrainingParametersKey, parametersBytes); err != nil {
		return nil, err
	}
	initialStatus := modeltraining.ModelStatus{Status: "training", Statistics: &modeltraining.ModelTrainingStatistics{Stage: "fetching data"}}
	if len(modelTrainingParameters) == 0 {
		initialStatus = modeltraining.ModelStatus{Status: "no model trained"}
	}
	_, err = p.PutModelTrainingStatus(ctx, &initialStatus)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to put model training status: %v", err)
	}

	resp, err := p.requestModelTraining(ctx, parametersBytes)
	if err != nil { // this fails if the request never made it to the training container
		// therefore we make a best effort to purge the stateful information associated
		// with the modeltraining request
		ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
		defer ca()
		delErr := p.deleteTrainingJobInfo(ctxca)
		return nil, errors.Join(err, delErr)
	}

	return resp, nil
}

func (p *AIOpsPlugin) LaunchAIServices(ctx context.Context) (*emptypb.Empty, error) {
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
	if _, err := p.PutAISettings(ctx, aiSettings); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *AIOpsPlugin) PutModelTrainingStatus(ctx context.Context, in *modeltraining.ModelStatus) (*emptypb.Empty, error) {
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
	if _, err := statisticsKv.Put(statisticsModelTrainingKey, bytesAggregation); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *AIOpsPlugin) deleteModelTrainingStatus(ctx context.Context) error {
	p.Logger.Info("Deleting model training status...")
	statisticsKv, err := p.statisticsKv.GetContext(ctx)
	if err != nil {
		return err
	}
	return statisticsKv.Delete(statisticsModelTrainingKey)
}

func (p *AIOpsPlugin) deleteModelTrainingParams(ctx context.Context) error {
	p.Logger.Info("Deleting model training parameters...")
	modelTrainingKv, err := p.modelTrainingKv.GetContext(ctx)
	if err != nil {
		return err
	}
	return modelTrainingKv.Delete(modelTrainingParametersKey)
}

// deletes stateful information associated with the model training
func (p *AIOpsPlugin) deleteTrainingJobInfo(ctx context.Context) error {
	return errors.Join(
		p.deleteModelTrainingStatus(ctx),
		p.deleteModelTrainingParams(ctx),
	)
}

func (p *AIOpsPlugin) ClusterWorkloadAggregation(ctx context.Context, in *corev1.Reference) (*modeltraining.WorkloadAggregationList, error) {
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

func (p *AIOpsPlugin) GetModelStatus(ctx context.Context, _ *emptypb.Empty) (*modeltraining.ModelStatus, error) {
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	statisticsKv, err := p.statisticsKv.GetContext(ctxca)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get model training status from Jetstream: %v", err)
	}
	result, err := statisticsKv.Get(statisticsModelTrainingKey)
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

func (p *AIOpsPlugin) GetModelTrainingParameters(ctx context.Context, _ *emptypb.Empty) (*modeltraining.ModelTrainingParametersList, error) {
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
