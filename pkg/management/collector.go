package management

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
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
		if nameLabel, ok := labels[corev1.NameLabel]; ok {
			friendlyName = nameLabel
		} else if legacyNameLabel, ok := labels[corev1.LegacyNameLabel]; ok {
			friendlyName = legacyNameLabel
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
