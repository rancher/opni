package model_training

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"
)

func (s *ModelTrainingPlugin) run_aggregation() {
	for {
		var r map[string]interface{}
		var cluster_namespace_deployment_aggregations = map[string]map[string]map[string]int{}
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
		for {
			var buf bytes.Buffer
			//var r map[string]interface{}
			if err := json.NewEncoder(&buf).Encode(request); err != nil {
				fmt.Println(err)

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
			if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
				log.Fatalf("Error parsing the response body: %s", err)
			}
			bucket_results := (r["aggregations"]).(map[string]interface{})["bucket"]
			bucket_aggregations := bucket_results.(map[string]interface{})["buckets"]
			result := reflect.ValueOf(bucket_aggregations)
			for i := 0; i < result.Len(); i++ {
				each_result := result.Index(i).Interface()
				doc_count := int((each_result.(map[string]interface{})["doc_count"]).(float64))
				key_results := (each_result.(map[string]interface{})["key"]).(map[string]interface{})
				cluster_id := (key_results["cluster_id"]).(string)
				namespace_name := (key_results["namespace_name"]).(string)
				deployment_name := (key_results["deployment_name"]).(string)
				_, cluster_found := cluster_namespace_deployment_aggregations[cluster_id]
				if cluster_found {
					_, namespace_found := cluster_namespace_deployment_aggregations[cluster_id][namespace_name]
					if namespace_found {
						cluster_namespace_deployment_aggregations[cluster_id][namespace_name][deployment_name] = doc_count
					} else {
						cluster_namespace_deployment_aggregations[cluster_id][namespace_name] = map[string]int{}
						cluster_namespace_deployment_aggregations[cluster_id][namespace_name][deployment_name] = doc_count
					}
				} else {
					cluster_namespace_deployment_aggregations[cluster_id] = map[string]map[string]int{}
					cluster_namespace_deployment_aggregations[cluster_id][namespace_name] = map[string]int{}
					cluster_namespace_deployment_aggregations[cluster_id][namespace_name][deployment_name] = doc_count
				}
			}
			after_key, after_key_present := bucket_results.(map[string]interface{})["after_key"]
			if after_key_present {
				((request["aggs"].(map[string]interface{})["bucket"]).(map[string]interface{})["composite"]).(map[string]interface{})["after"] = after_key
			} else {
				break
			}
		}
		aggregated_results, _ := json.Marshal(cluster_namespace_deployment_aggregations)
		bytes_aggregation := []byte(aggregated_results)
		s.kv.Get().Put("aggregation", bytes_aggregation)
		s.Logger.Info("Updated aggregation of deployments to Jetstream.")
		time.Sleep(30 * time.Second)
	}
}
