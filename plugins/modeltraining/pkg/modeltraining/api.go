package modeltraining

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	modeltraining "github.com/rancher/opni/plugins/modeltraining/pkg/apis/modeltraining"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
)

type ModelTrainingParameters struct {
	ByCluster map[string]*ClusterParameters `json:",inline,omitEmpty"`
}

type ClusterParameters struct {
	ByNamespace map[string]*NamespaceParameters `json:",inline,omitEmpty"`
}

type NamespaceParameters struct {
	ByDeployment map[string]*Deployment `json:",inline,omitEmpty"`
}

type Deployment struct {
	DeploymentName string `json:",inline,omitEmpty"`
}

func (c *ModelTrainingPlugin) TrainModel(ctx context.Context, in *modeltraining.WorkloadsList) (*corev1.Reference, error) {
	var modelTrainingParameters = map[string]map[string][]string{}
	for _, item := range in.List {
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

func (c *ModelTrainingPlugin) WorkloadLogCount(ctx context.Context, in *corev1.Reference) (*modeltraining.WorkloadsList, error) {
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
		return &modeltraining.WorkloadsList{}, nil
	}
	workloadsList := modeltraining.WorkloadsList{}
	workloadArray := make([]*modeltraining.WorkloadResponse, 0)
	for namespaceName, deployments := range clusterAggregationResults.ByNamespace {
		for deploymentName, count := range deployments.ByDeployment {
			workloadAggregation := modeltraining.WorkloadResponse{ClusterId: in.Id, Namespace: namespaceName, Deployment: deploymentName, Count: int64(count.Count)}
			workloadArray = append(workloadArray, &workloadAggregation)
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

func (c *ModelTrainingPlugin) ModelTrainingParameters(ctx context.Context, in *emptypb.Empty) (*modeltraining.WorkloadsList, error) {
	b := []byte("model_training_parameters")
	msg, err := c.natsConnection.Get().Request("workload_parameters", b, time.Minute)
	if err != nil {
		return nil, err
	}
	trainingParameters := modeltraining.WorkloadsList{}
	parametersArray := make([]*modeltraining.WorkloadResponse, 0)
	var resultsStorage = map[string]map[string][]string{}
	if err := json.Unmarshal(msg.Data, &resultsStorage); err != nil {
		return nil, err
	}
	for clusterName, namespaces := range resultsStorage {
		for namespaceName, deployments := range namespaces {
			if deployments == nil {
				c.Logger.Error("Unexpected nil deployment for array.")
				continue
			}
			for deploymentIdx := range deployments {
				deploymentData := modeltraining.WorkloadResponse{ClusterId: clusterName, Namespace: namespaceName, Deployment: deployments[deploymentIdx]}
				parametersArray = append(parametersArray, &deploymentData)
			}
		}
	}
	trainingParameters.List = parametersArray
	return &trainingParameters, nil
}

func (c *ModelTrainingPlugin) GpuPresentCluster(ctx context.Context, in *emptypb.Empty) (*modeltraining.GPUInfoList, error) {

	nodes := &k8scorev1.NodeList{}
	if err := c.k8sClient.Get().List(ctx, nodes); err != nil {
		return nil, err
	}
	returnedData := modeltraining.GPUInfoList{}
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
	returnedData.List = gpuInfoArray
	return &returnedData, nil
}