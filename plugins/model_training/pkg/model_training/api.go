package model_training

import (
	"context"

	model_training "github.com/rancher/opni/plugins/model_training/pkg/apis/model_training"
)

func (c *ModelTrainingPlugin) ListWorkloads(ctx context.Context, in *model_training.ClusterID) (*model_training.WorkloadResponse, error) {
	/*
		tester, _ := js.KeyValue("os-workload-aggregation")
		fmt.Println(tester)
		fmt.Println(tester.Keys())
		result, _ := kv.Get("aggregation")
		jsonRes := result.Value()
	*/
	return nil, nil
}
