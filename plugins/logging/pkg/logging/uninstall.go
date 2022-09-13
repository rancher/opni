package logging

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/opensearch-project/opensearch-go"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pendingValue = "job pending"
)

type deleteStatus int

const (
	deletePending deleteStatus = iota
	deleteRunning
	deleteFinished
	deleteFinishedWithErrors
	deleteError
)

type UninstallTaskRunner struct {
	uninstall.DefaultPendingHandler
	storageNamespace string
	kv               nats.KeyValue
	opensearchClient future.Future[*opensearch.Client]
	natsConnection   *nats.Conn
	k8sClient        client.Client
	storageBackend   future.Future[storage.Backend]
}

func (a *UninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	var md uninstall.TimestampedMetadata
	ti.LoadTaskMetadata(&md)

	ti.AddLogEntry(zapcore.InfoLevel, "Uninstalling logging capability for this cluster")

	if md.DeleteStoredData {
		ti.AddLogEntry(zapcore.WarnLevel, "Will delete opensearch data")
		err := a.getPendingDeleteBucket()
		if err != nil {
			return err
		}
		if err := a.doClusterDataDelete(ctx, ti.TaskId()); err != nil {
			return err
		}

		ti.AddLogEntry(zapcore.InfoLevel, "Delete request accepted; polling status")

		p := backoff.Exponential(
			backoff.WithMaxRetries(0),
			backoff.WithMinInterval(5*time.Second),
			backoff.WithMaxInterval(1*time.Minute),
			backoff.WithMultiplier(1.1),
		)
		b := p.Start(ctx)
	RETRY:
		for {
			select {
			case <-b.Done():
				ti.AddLogEntry(zapcore.WarnLevel, "Uninstall canceled, but time series data is still being deleted by Cortex")
				return ctx.Err()
			case <-b.Next():
				status, err := a.deleteTaskStatus(ctx, ti.TaskId())
				if err != nil {
					ti.AddLogEntry(zapcore.ErrorLevel, err.Error())
					continue
				}
				switch status {
				case deletePending:
					if err := a.doClusterDataDelete(ctx, ti.TaskId()); err != nil {
						ti.AddLogEntry(zapcore.ErrorLevel, err.Error())
						continue
					}
				case deleteFinishedWithErrors:
					ti.AddLogEntry(zapcore.WarnLevel, "some log entries not deleted")
					break RETRY
				case deleteFinished:
					break RETRY
				}
			}
		}
	} else {
		ti.AddLogEntry(zapcore.InfoLevel, "Log data will not be deleted")
	}

	err := a.deleteKubernetesObjects(ctx, ti.TaskId())
	if err != nil {
		return err
	}

	ti.AddLogEntry(zapcore.InfoLevel, "Removing capability from cluster metadata")
	_, err = a.storageBackend.Get().UpdateCluster(ctx, &opnicorev1.Reference{
		Id: ti.TaskId(),
	}, storage.NewRemoveCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)))
	if err != nil {
		return err
	}
	return nil
}

func (a *UninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
	switch state {
	case task.StateCompleted:
		ti.AddLogEntry(zapcore.InfoLevel, "Capability uninstalled successfully")
		return // no deletion timestamp to reset, since the capability should be gone
	case task.StateFailed:
		ti.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("Capability uninstall failed: %v", args[0]))
	case task.StateCanceled:
		ti.AddLogEntry(zapcore.InfoLevel, "Capability uninstall canceled")
	}

	// Reset the deletion timestamp
	_, err := a.storageBackend.Get().UpdateCluster(ctx, &opnicorev1.Reference{
		Id: ti.TaskId(),
	}, func(c *opnicorev1.Cluster) {
		for _, cap := range c.GetCapabilities() {
			if cap.Name == wellknown.CapabilityLogs {
				cap.DeletionTimestamp = nil
			}
		}
	})
	if err != nil {
		ti.AddLogEntry(zapcore.WarnLevel, fmt.Sprintf("Failed to reset deletion timestamp: %v", err))
	}
}

func (a *UninstallTaskRunner) deleteKubernetesObjects(ctx context.Context, id string) error {
	var (
		loggingCluster *opnicorev1beta1.LoggingCluster
		secret         *corev1.Secret
	)

	loggingClusterList := &opnicorev1beta1.LoggingClusterList{}
	if err := a.k8sClient.List(
		ctx,
		loggingClusterList,
		client.InNamespace(a.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		ErrListingClustersFaled(err)
	}
	if len(loggingClusterList.Items) > 1 {
		return ErrDeleteClusterInvalidList(id)
	}
	if len(loggingClusterList.Items) == 1 {
		loggingCluster = &loggingClusterList.Items[0]
	}

	secretList := &corev1.SecretList{}
	if err := a.k8sClient.List(
		ctx,
		secretList,
		client.InNamespace(a.storageNamespace),
		client.MatchingLabels{resources.OpniClusterID: id},
	); err != nil {
		ErrListingClustersFaled(err)
	}

	if len(secretList.Items) > 1 {
		return ErrDeleteClusterInvalidList(id)
	}
	if len(secretList.Items) == 1 {
		secret = &secretList.Items[0]
	}

	if loggingCluster != nil {
		if err := a.k8sClient.Delete(ctx, loggingCluster); err != nil {
			return err
		}
	}

	return a.k8sClient.Delete(ctx, secret)
}

func (a *UninstallTaskRunner) getPendingDeleteBucket() error {
	mgr, err := a.natsConnection.JetStream()
	if err != nil {
		return err
	}

	a.kv, err = mgr.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "pending-delete",
		Description: "track pending deletes",
	})
	return err
}

func (a *UninstallTaskRunner) doClusterDataDelete(ctx context.Context, id string) error {
	var createNewJob bool
	idExists, err := a.keyExists(id)
	if err != nil {
		return nil
	}

	if idExists {
		entry, err := a.kv.Get(id)
		if err != nil {
			return nil
		}
		createNewJob = !(string(entry.Value()) == pendingValue)
	} else {
		createNewJob = true
	}

	query, _ := sjson.Set("", `query.term.cluster_id`, id)
	if createNewJob {
		_, err = a.kv.PutString(id, pendingValue)
		if err != nil {
			return err
		}

		resp, err := a.opensearchClient.Get().DeleteByQuery(
			[]string{
				"logs",
			},
			strings.NewReader(query),
			a.opensearchClient.Get().DeleteByQuery.WithWaitForCompletion(false),
			a.opensearchClient.Get().DeleteByQuery.WithRefresh(true),
			a.opensearchClient.Get().DeleteByQuery.WithSearchType("dfs_query_then_fetch"),
			a.opensearchClient.Get().DeleteByQuery.WithContext(ctx),
		)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.IsError() {
			return ErrOpensearchRequestFailed(resp.String())
		}

		respString := util.ReadString(resp.Body)
		taskID := gjson.Get(respString, "task").String()
		_, err = a.kv.PutString(id, taskID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *UninstallTaskRunner) keyExists(keyToCheck string) (bool, error) {
	keys, err := a.kv.Keys()
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		if key == keyToCheck {
			return true, nil
		}
	}
	return false, nil
}

func (a *UninstallTaskRunner) deleteTaskStatus(ctx context.Context, id string) (deleteStatus, error) {
	value, err := a.kv.Get(id)
	if err != nil {
		return deleteError, err
	}

	taskID := string(value.Value())

	if taskID == pendingValue {
		return deletePending, nil
	}

	resp, err := a.opensearchClient.Get().Tasks.Get(
		taskID,
		a.opensearchClient.Get().Tasks.Get.WithContext(ctx),
	)
	if err != nil {
		return deleteError, err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return deleteError, ErrOpensearchRequestFailed(resp.String())
	}

	body := util.ReadString(resp.Body)

	if !gjson.Get(body, "completed").Bool() {
		return deleteRunning, nil
	}

	if len(gjson.Get(body, "response.failures").Array()) > 0 {
		return deleteFinishedWithErrors, nil
	}

	return deleteFinished, nil
}
