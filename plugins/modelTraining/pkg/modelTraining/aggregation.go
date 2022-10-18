package model_training

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
	return &Aggregations{ByCluster: make(map[string]*ClusterAggregation)}
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

func (s *ModelTrainingPlugin) run_aggregation() {
	for {
		request := map[string]interface{}{
			"size": 0,
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"must": []map[string]interface{}{
						{
							"match": map[string]interface{}{
								"log_type": "workload",
							},
						},
						{
							"regexp": map[string]interface{}{
								"kubernetes.namespace_name.keyword": ".+",
							},
						},
						{
							"regexp": map[string]interface{}{
								"deployment.keyword": ".+",
							},
						},
					},
				},
			},
			"aggs": map[string]interface{}{
				"bucket": map[string]interface{}{
					"composite": map[string]interface{}{
						"size": 4,
						"sources": []map[string]interface{}{
							{
								"cluster_id": map[string]interface{}{
									"terms": map[string]interface{}{
										"field": "cluster_id",
									},
								},
							},
							{
								"namespace_name": map[string]interface{}{
									"terms": map[string]interface{}{
										"field": "kubernetes.namespace_name.keyword",
									},
								},
							},
							{
								"deployment_name": map[string]interface{}{
									"terms": map[string]interface{}{
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
			//var r map[string]interface{}
			if err := json.NewEncoder(&buf).Encode(request); err != nil {
				log.Fatalf("Error getting response: %s", err)

			}
			res, err := s.osClient.Get().Search(
				s.osClient.Get().Search.WithContext(context.Background()),
				s.osClient.Get().Search.WithIndex("logs"),
				s.osClient.Get().Search.WithBody(&buf),
				s.osClient.Get().Search.WithTrackTotalHits(true),
				s.osClient.Get().Search.WithPretty(),
			)
			if err != nil {
				log.Fatalf("Error getting response: %s", err)
			}
			defer res.Body.Close()
			var result SearchResponse
			if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
				log.Fatalf("Error parsing the response body: %s", err)
			}
			for _, b := range result.Aggregations.Bucket.Buckets {
				resultAgg.Add(b)
			}
			after_key := result.Aggregations.Bucket.AfterKey
			if after_key != nil {
				((request["aggs"].(map[string]interface{})["bucket"]).(map[string]interface{})["composite"]).(map[string]interface{})["after"] = after_key
			} else {
				break
			}
		}
		aggregated_results, _ := json.Marshal(resultAgg)
		bytes_aggregation := []byte(aggregated_results)
		s.kv.Get().Put("aggregation", bytes_aggregation)
		s.Logger.Info("Updated aggregation of deployments to Jetstream.")
		time.Sleep(30 * time.Second)
	}
}
