package management

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
)

var (
	clusterInfo = prometheus.NewDesc(
		"opni_monitoring_cluster_info",
		"Cluster information",
		[]string{"cluster_id", "friendly_name"},
		prometheus.Labels{},
	)
)

func (s *Server) Describe(c chan<- *prometheus.Desc) {
	c <- clusterInfo
}

func (s *Server) Collect(c chan<- prometheus.Metric) {
	ctx, ca := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer ca()
	clusters, err := s.ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return
	}
	for _, cluster := range clusters.Items {
		var friendlyName = cluster.Id
		labels := cluster.GetLabels()
		// todo: this label should change
		if nameLabel, ok := labels["kubernetes.io/metadata.name"]; ok {
			friendlyName = nameLabel
		}
		c <- prometheus.MustNewConstMetric(
			clusterInfo,
			prometheus.GaugeValue,
			1,
			cluster.Id,
			friendlyName,
		)
	}
}
