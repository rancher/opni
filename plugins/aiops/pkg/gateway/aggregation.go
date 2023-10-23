package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type Key struct {
	ClusterID      string `json:"cluster_id"`
	NamespaceName  string `json:"namespace_name"`
	DeploymentName string `json:"deployment_name"`
}

type SearchResponse struct {
	Aggregations AggregationsSpec `json:"aggregations"`
}

type AggregationsSpec struct {
	Bucket BucketSpec `json:"bucket"`
}

type BucketSpec struct {
	AfterKey *Key     `json:"after_key"`
	Buckets  []Bucket `json:"buckets"`
}

type Bucket struct {
	Key      Key `json:"key"`
	DocCount int `json:"doc_count"`
}

type Aggregations struct {
	ByCluster map[string]*ClusterAggregation `json:",inline,omitEmpty"`
}

type ClusterAggregation struct {
	ByNamespace map[string]*NamespaceAggregation `json:",inline,omitEmpty"`
}

type NamespaceAggregation struct {
	ByDeployment map[string]*DeploymentLogCount `json:",inline,omitEmpty"`
}

type DeploymentLogCount struct {
	DeploymentName string `json:"deployment_name"`
	Count          int    `json:"doc_count"`
}

func newAggregations() *Aggregations {
	return &Aggregations{
		ByCluster: make(map[string]*ClusterAggregation),
	}
}

func (a *Aggregations) Add(bucket Bucket) {
	if _, ok := a.ByCluster[bucket.Key.ClusterID]; !ok {
		a.ByCluster[bucket.Key.ClusterID] = &ClusterAggregation{
			ByNamespace: make(map[string]*NamespaceAggregation),
		}
	}
	cluster := a.ByCluster[bucket.Key.ClusterID]
	if _, ok := cluster.ByNamespace[bucket.Key.NamespaceName]; !ok {
		cluster.ByNamespace[bucket.Key.NamespaceName] = &NamespaceAggregation{
			ByDeployment: make(map[string]*DeploymentLogCount),
		}
	}
	a.ByCluster[bucket.Key.ClusterID].ByNamespace[bucket.Key.NamespaceName].ByDeployment[bucket.Key.DeploymentName] = &DeploymentLogCount{
		DeploymentName: bucket.Key.DeploymentName,
		Count:          bucket.DocCount,
	}
}

func (p *AIOpsPlugin) aggregateWorkloadLogs() {
	request := map[string]any{
		"size": 0,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []map[string]any{
					{
						"match": map[string]any{
							"log_type": "workload",
						},
					},
					{
						"regexp": map[string]any{
							"namespace_name.keyword": ".+",
						},
					},
					{
						"regexp": map[string]any{
							"deployment.keyword": ".+",
						},
					},
				},
			},
		},
		"aggs": map[string]any{
			"bucket": map[string]any{
				"composite": map[string]any{
					"size": 1000,
					"sources": []map[string]any{
						{
							"cluster_id": map[string]any{
								"terms": map[string]any{
									"field": "cluster_id",
								},
							},
						},
						{
							"namespace_name": map[string]any{
								"terms": map[string]any{
									"field": "namespace_name.keyword",
								},
							},
						},
						{
							"deployment_name": map[string]any{
								"terms": map[string]any{
									"field": "deployment.keyword",
								},
							},
						},
					},
				},
			},
		},
	}
	resultAgg := newAggregations()
	for {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(request); err != nil {
			p.Logger.Error(fmt.Sprintf("Error: Unable to encode request: %s", err))
			return
		}
		res, err := p.osClient.Get().Search(
			p.osClient.Get().Search.WithContext(p.ctx),
			p.osClient.Get().Search.WithIndex("logs"),
			p.osClient.Get().Search.WithBody(&buf),
			p.osClient.Get().Search.WithTrackTotalHits(true),
			p.osClient.Get().Search.WithPretty(),
		)
		if err != nil {
			p.Logger.Error(fmt.Sprintf("Unable to connect to Opensearch %s", err))
			return
		}
		defer res.Body.Close()
		if res.IsError() {
			p.Logger.Error(fmt.Sprintf("Error: %s", res.String()))
			return
		}
		var result SearchResponse
		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			p.Logger.Error(fmt.Sprintf("Error parsing the response body: %s", err))
			return
		}
		for _, b := range result.Aggregations.Bucket.Buckets {
			resultAgg.Add(b)
		}
		afterKey := result.Aggregations.Bucket.AfterKey
		if afterKey != nil {
			((request["aggs"].(map[string]any)["bucket"]).(map[string]any)["composite"]).(map[string]any)["after"] = afterKey
		} else {
			break
		}
	}
	aggregatedResults, err := json.Marshal(resultAgg)
	if err != nil {
		p.Logger.Error(fmt.Sprintf("Error: %s", err))
		return
	}
	bytesAggregation := []byte(aggregatedResults)
	p.aggregationKv.Get().Put("aggregation", bytesAggregation)
	p.Logger.Info("Updated aggregation of deployments to Jetstream.")
}

func (p *AIOpsPlugin) runAggregation() {
	t := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-t.C:
			p.aggregateWorkloadLogs()
		case <-p.ctx.Done():
			t.Stop()
			return
		}
	}
}
