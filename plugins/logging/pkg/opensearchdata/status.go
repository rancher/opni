package opensearchdata

import (
	"context"

	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/rancher/opni/pkg/util"
	"github.com/tidwall/gjson"
)

func (m *Manager) GetClusterStatus() ClusterStatus {
	m.WaitForInit()
	m.Lock()
	defer m.Unlock()

	req := opensearchapi.ClusterHealthRequest{}
	resp, err := req.Do(context.TODO(), m.Client)
	if err != nil {
		m.logger.With("err", err).Error("failed to fetch opensearch cluster status")
		return ClusterStatusError
	}
	defer resp.Body.Close()

	if resp.IsError() {
		m.logger.With("resp", resp.String).Error("failure response from cluster status")
		return ClusterStatusError
	}

	respString := util.ReadString(resp.Body)
	status := gjson.Get(respString, "status").String()
	switch status {
	case "green":
		return ClusterStatusGreen
	case "yellow":
		return ClusterStatusYellow
	case "red":
		return ClusterStatusRed
	default:
		m.logger.Errorf("unknown status: %s", status)
		return ClusterStatusError
	}
}
