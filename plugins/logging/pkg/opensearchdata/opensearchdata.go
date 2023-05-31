package opensearchdata

import (
	"context"
	"fmt"
	"sync"

	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/util/future"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"go.uber.org/zap"
)

const (
	pendingValue     = "job pending"
	opensearchPrefix = "os_"
)

type DeleteStatus int

const (
	DeletePending DeleteStatus = iota
	DeleteRunning
	DeleteFinished
	DeleteFinishedWithErrors
	DeleteError
)

type ClusterStatus int

// Ready func should return true if there is a critical error
// That would stop the opensearch query from running.
type ReadyFunc func() bool

const (
	ClusterStatusGreen = iota
	ClusterStatusYellow
	ClusterStatusRed
	ClusterStatusError
	ClusterStatusNoClient
)

type Manager struct {
	*loggingutil.AsyncOpensearchClient

	systemKV future.Future[system.KeyValueStoreClient]
	logger   *zap.SugaredLogger

	adminInitStateRW sync.RWMutex
}

func NewManager(logger *zap.SugaredLogger, kv future.Future[system.KeyValueStoreClient]) *Manager {
	return &Manager{
		AsyncOpensearchClient: loggingutil.NewAsyncOpensearchClient(),
		systemKV:              kv,
		logger:                logger,
	}
}

func (m *Manager) keyExists(keyToCheck string) (bool, error) {
	prefixKey := &system.Key{
		Key: opensearchPrefix,
	}
	keys, err := m.systemKV.Get().ListKeys(context.Background(), prefixKey)
	if err != nil {
		return false, err
	}
	for _, key := range keys.GetItems() {
		if key == fmt.Sprintf("%s%s", opensearchPrefix, keyToCheck) {
			return true, nil
		}
	}
	return false, nil
}
