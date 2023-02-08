package opensearchdata

import (
	"context"
	"strings"

	"github.com/rancher/opni/pkg/util"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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

	m.kv.SetClient(m.setJetStream, false)
	m.kv.WaitForInit()

	var createNewJob bool
	idExists, err := m.keyExists(id)
	if err != nil {
		return nil
	}

	if idExists {
		entry, err := m.kv.Client.Get(id)
		if err != nil {
			return nil
		}
		createNewJob = string(entry.Value()) == pendingValue
	} else {
		createNewJob = true
	}

	query, _ := sjson.Set("", `query.term.cluster_id`, id)
	if createNewJob {
		if idExists {
			_, err = m.kv.Client.PutString(id, pendingValue)
		} else {
			_, err = m.kv.Client.Create(id, []byte(pendingValue))
		}
		if err != nil {
			return err
		}

		resp, err := m.Client.Indices.AsyncDeleteByQuery(ctx, []string{"logs"}, strings.NewReader(query))
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.IsError() {
			return loggingerrors.ErrOpensearchRequestFailed(resp.String())
		}

		respString := util.ReadString(resp.Body)
		taskID := gjson.Get(respString, "task").String()
		m.logger.Debugf("opensearch taskID is :%s", taskID)
		_, err = m.kv.Client.PutString(id, taskID)
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

	m.kv.WaitForInit()

	idExists, err := m.keyExists(id)
	if err != nil {
		return DeleteError, err
	}
	// If ID doesn't exist in KV set task to finished with errors
	if !idExists {
		m.logger.Warn("could not find cluster id in KV store")
		return DeleteFinishedWithErrors, nil
	}

	value, err := m.kv.Client.Get(id)
	if err != nil {
		return DeleteError, err
	}

	taskID := string(value.Value())

	if taskID == pendingValue {
		m.logger.Debug("kv status is pending")
		return DeletePending, nil
	}

	resp, err := m.Client.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return DeleteError, err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return DeleteError, loggingerrors.ErrOpensearchRequestFailed(resp.String())
	}

	body := util.ReadString(resp.Body)

	if !gjson.Get(body, "completed").Bool() {
		m.logger.Debug(body)
		return DeleteRunning, nil
	}

	if len(gjson.Get(body, "response.failures").Array()) > 0 {
		return DeleteFinishedWithErrors, nil
	}

	err = m.kv.Client.Delete(id)
	if err != nil {
		return DeleteError, err
	}

	return DeleteFinished, nil
}
