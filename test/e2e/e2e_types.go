package e2e

import "time"

type SearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore float64 `json:"max_score"`
		Hits     []struct {
			Index  string  `json:"_index"`
			Type   string  `json:"_type"`
			ID     string  `json:"_id"`
			Score  float64 `json:"_score"`
			Source struct {
				Log                         string    `json:"log"`
				Time                        time.Time `json:"time"`
				TimeNanoseconds             int64     `json:"time_nanoseconds"`
				WindowDt                    int64     `json:"window_dt"`
				WindowStartTimeNs           int64     `json:"window_start_time_ns"`
				MaskedLog                   string    `json:"masked_log"`
				Timestamp                   time.Time `json:"timestamp"`
				IsControlPlaneLog           bool      `json:"is_control_plane_log"`
				KubernetesComponent         string    `json:"kubernetes_component"`
				AnomalyPredictedCount       float64   `json:"anomaly_predicted_count"`
				NulogAnomaly                bool      `json:"nulog_anomaly"`
				DrainAnomaly                bool      `json:"drain_anomaly"`
				NulogConfidence             float64   `json:"nulog_confidence"`
				DrainMatchedTemplateID      float64   `json:"drain_matched_template_id"`
				DrainMatchedTemplateSupport float64   `json:"drain_matched_template_support"`
				AnomalyLevel                string    `json:"anomaly_level"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type CountResponse struct {
	Count  int `json:"count"`
	Shards struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
}
