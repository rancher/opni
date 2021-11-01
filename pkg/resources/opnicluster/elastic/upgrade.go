package elastic

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ClusterHealthResponse struct {
	Status             string  `json:"status,omitempty"`
	ActiveShards       int     `json:"active_shards,omitempty"`
	RelocatingShards   int     `json:"relocating_shards,omitempty"`
	InitializingShards int     `json:"initializing_shards,omitempty"`
	UnassignedShards   int     `json:"unassigned_shards,omitempty"`
	PercentActive      float32 `json:"active_shards_percent_as_number,omitempty"`
}

type ClusterSettings struct {
	Persistent Persistent `json:"persistent,omitempty"`
}
type Persistent struct {
	ClusterRoutingAllocationEnable string `json:"cluster.routing.allocation.enable,omitempty"`
}

func (r *Reconciler) UpgradeData() (retry bool, err error) {
	statefulset := appsv1.StatefulSet{}
	err = r.client.Get(r.ctx, types.NamespacedName{
		Name:      OpniDataWorkload,
		Namespace: r.opniCluster.Namespace,
	}, &statefulset)
	if err != nil {
		return false, err
	}

	// If not all nodes are ready requeue
	if statefulset.Status.ReadyReplicas != statefulset.Status.Replicas {
		return true, nil
	}

	// create the client
	err = r.createClient()
	if err != nil {
		return
	}

	// Check if all shards are green
	if !r.areShardsGreen() {
		// Check settings
		getReq := opensearchapi.ClusterGetSettingsRequest{
			FlatSettings: pointer.BoolPtr(true),
		}
		resp, err := getReq.Do(r.ctx, r.esClient)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return false, fmt.Errorf("failed to check cluster settings: %s", resp.String())
		}

		settings := ClusterSettings{}
		err = json.NewDecoder(resp.Body).Decode(&settings)
		if err != nil {
			return false, err
		}

		// return if settings are already set correctly
		if settings.Persistent.ClusterRoutingAllocationEnable == "all" {
			return true, nil
		}

		putReq := opensearchapi.ClusterPutSettingsRequest{
			Body: opensearchutil.NewJSONReader(ClusterSettings{
				Persistent: Persistent{
					ClusterRoutingAllocationEnable: "all",
				},
			}),
			FlatSettings: pointer.BoolPtr(true),
		}
		resp, err = putReq.Do(r.ctx, r.esClient)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if resp.IsError() {
			return false, fmt.Errorf("failed to put cluster settings: %s", resp.String())
		}

		return true, nil
	}

	// If upgradde is complete return without requeue
	if statefulset.Status.CurrentRevision == statefulset.Status.UpdateRevision {
		return
	}

	// The above logic doesn't work due to https://github.com/kubernetes/kubernetes/issues/73492 so work around it in the meantime
	if statefulset.Status.UpdatedReplicas == statefulset.Status.Replicas {
		return
	}

	putReq := opensearchapi.ClusterPutSettingsRequest{
		Body: opensearchutil.NewJSONReader(ClusterSettings{
			Persistent: Persistent{
				ClusterRoutingAllocationEnable: "primaries",
			},
		}),
		FlatSettings: pointer.BoolPtr(true),
	}
	resp, err := putReq.Do(r.ctx, r.esClient)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return false, fmt.Errorf("failed to put cluster settings: %s", resp.String())
	}

	var deleteOrdinal int
	if r.opniCluster.Spec.Elastic.Workloads.Data.Replicas == nil {
		deleteOrdinal = 0
	} else {
		deleteOrdinal = int(*r.opniCluster.Spec.Elastic.Workloads.Data.Replicas) - 1 - int(statefulset.Status.UpdatedReplicas)
	}

	r.client.Delete(r.ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", OpniDataWorkload, deleteOrdinal),
			Namespace: r.opniCluster.Namespace,
		},
	})

	return true, nil
}

func (r *Reconciler) createClient() error {
	// Fetch the admin password
	var password string
	if r.opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef != nil {
		secret := &corev1.Secret{}
		if err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef.Name,
			Namespace: r.opniCluster.Namespace,
		}, secret); err != nil {
			return err
		}
		password = string(secret.Data[r.opniCluster.Status.Auth.ElasticsearchAuthSecretKeyRef.Key])
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	r.esClient, _ = opensearch.NewClient(opensearch.Config{
		Addresses: []string{
			fmt.Sprintf("https://opni-es-client.%s:9200", r.opniCluster.Namespace),
		},
		Username:             "admin",
		Password:             password,
		UseResponseCheckOnly: true,
		Transport:            transport,
	})
	return nil
}

func (r *Reconciler) areShardsGreen() bool {
	lg := log.FromContext(r.ctx)

	req := opensearchapi.ClusterHealthRequest{
		Timeout:       10 * time.Second,
		WaitForStatus: "green",
	}
	resp, err := req.Do(r.ctx, r.esClient)
	if err != nil {
		lg.Error(err, "failed to fetch cluster status")
		return false
	}
	defer resp.Body.Close()
	if resp.IsError() {
		lg.Error(fmt.Errorf("%s", resp.String()), "failed to fetch cluster status")
		return false
	}

	health := ClusterHealthResponse{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	if err != nil {
		lg.Error(err, "failed to decode cluster status")
		return false
	}

	lg.V(1).Info(fmt.Sprintf("%.2f percent of shards ready", health.PercentActive))
	return health.Status == "green"
}
