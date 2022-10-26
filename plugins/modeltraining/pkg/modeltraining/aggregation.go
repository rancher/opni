package modeltraining

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
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

func (s *ModelTrainingPlugin) aggregateWorkloadLogs() {
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
							"kubernetes.namespace_name.keyword": ".+",
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
					"size": 4,
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
									"field": "kubernetes.namespace_name.keyword",
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
			log.Fatalf("Error: Unable to encode request: %s", err)

		}
		res, err := s.osClient.Get().Search(
			s.osClient.Get().Search.WithContext(context.Background()),
			s.osClient.Get().Search.WithIndex("logs"),
			s.osClient.Get().Search.WithBody(&buf),
			s.osClient.Get().Search.WithTrackTotalHits(true),
			s.osClient.Get().Search.WithPretty(),
		)
		if err != nil {
			s.Logger.Fatalf("Unable to connect to Opensearch %s", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			s.Logger.Fatalf("Error: %s", res.String())
		}
		var result SearchResponse
		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			s.Logger.Fatalf("Error parsing the response body: %s", err)
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
		s.Logger.Fatalf("Error: %s", err)
	}
	bytesAggregation := []byte(aggregatedResults)
	s.kv.Get().Put("aggregation", bytesAggregation)
	s.Logger.Info("Updated aggregation of deployments to Jetstream.")
}

func (s *ModelTrainingPlugin) runAggregation() {
	t := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-t.C:
			s.aggregateWorkloadLogs()
		case <-s.ctx.Done():
			t.Stop()
			return
		}
	}
}
