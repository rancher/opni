package opensearchdata

import (
	"context"
	"fmt"
	"strings"

	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/util"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TODO: Tracking of external system task state should be generalized and moved
// to the generic task controller

func (m *Manager) DoClusterDataDelete(ctx context.Context, id string, readyFunc ...ReadyFunc) error {
	m.WaitForInit()

	for _, r := range readyFunc {
		exitEarly := r()
		if exitEarly {
			m.logger.Warn("opensearch cluster is never able to receive queries")
			return nil
		}

	}

	m.Lock()
	defer m.Unlock()

	var createNewJob bool
	idExists, err := m.keyExists(id)
	if err != nil {
		return nil
	}

	if idExists {
		entry, err := m.systemKV.Get().Get(ctx, &system.GetRequest{
			Key: fmt.Sprintf("%s%s", opensearchPrefix, id),
		})
		if err != nil {
			return nil
		}
		createNewJob = string(entry.GetValue()) == pendingValue
	} else {
		createNewJob = true
	}

	query, _ := sjson.Set("", `query.term.cluster_id`, id)
	if createNewJob {
		_, err := m.systemKV.Get().Put(ctx, &system.PutRequest{
			Key:   fmt.Sprintf("%s%s", opensearchPrefix, id),
			Value: []byte(pendingValue),
		})
		if err != nil {
			return err
		}

		resp, err := m.Client.Indices.AsyncDeleteByQuery(ctx, []string{"logs"}, strings.NewReader(query))
		if err != nil {
			return loggingerrors.WrappedOpensearchFailure(err)
		}
		defer resp.Body.Close()

		if resp.IsError() {
			m.logger.Error(fmt.Sprintf("opensearch request failed: %s", resp.String()))
			return loggingerrors.ErrOpensearchResponse
		}

		respString := util.ReadString(resp.Body)
		taskID := gjson.Get(respString, "task").String()
		m.logger.Debug(fmt.Sprintf("opensearch taskID is :%s", taskID))
		_, err = m.systemKV.Get().Put(ctx, &system.PutRequest{
			Key:   fmt.Sprintf("%s%s", opensearchPrefix, id),
			Value: []byte(taskID),
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) DeleteTaskStatus(ctx context.Context, id string, readyFunc ...ReadyFunc) (DeleteStatus, error) {
	m.WaitForInit()

	for _, r := range readyFunc {
		exitEarly := r()
		if exitEarly {
			m.logger.Warn("opensearch cluster is never able to receive queries")
			return DeleteFinishedWithErrors, nil
		}
	}

	m.Lock()
	defer m.Unlock()

	idExists, err := m.keyExists(id)
	if err != nil {
		return DeleteError, err
	}
	// If ID doesn't exist in KV set task to finished with errors
	if !idExists {
		m.logger.Warn("could not find cluster id in KV store")
		return DeleteFinishedWithErrors, nil
	}

	value, err := m.systemKV.Get().Get(ctx, &system.GetRequest{
		Key: fmt.Sprintf("%s%s", opensearchPrefix, id),
	})
	if err != nil {
		return DeleteError, err
	}

	taskID := string(value.GetValue())

	if taskID == pendingValue {
		m.logger.Debug("kv status is pending")
		return DeletePending, nil
	}

	resp, err := m.Client.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return DeleteError, err
	}
	defer resp.Body.Close()

	var status DeleteStatus
	body := util.ReadString(resp.Body)

	switch {
	// If the task does not exist we can never get the status
	// so consider it finished with errors
	case resp.StatusCode == 404:
		status = DeleteFinishedWithErrors
	case resp.IsError():
		return DeleteError, loggingerrors.ErrOpensearchResponse
	case !gjson.Get(body, "completed").Bool():
		m.logger.Debug(body)
		return DeleteRunning, nil
	case len(gjson.Get(body, "response.failures").Array()) > 0:
		status = DeleteFinishedWithErrors
	default:
		status = DeleteFinished
	}

	_, err = m.systemKV.Get().Delete(ctx, &system.DeleteRequest{
		Key: fmt.Sprintf("%s%s", opensearchPrefix, id),
	})
	if err != nil {
		return DeleteError, err
	}

	return status, nil
}
