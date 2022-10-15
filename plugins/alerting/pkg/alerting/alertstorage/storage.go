package alertstorage

import (
	"context"
	"path"
	"time"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/future"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"go.uber.org/zap"
)

const conditionPrefix = "/alerting/conditions"
const endpointPrefix = "/alerting/endpoints"

type StorageAPIs struct {
	Conditions storage.KeyValueStoreT[*alertingv1alpha.AlertCondition]
	Endpoints  storage.KeyValueStoreT[*alertingv1alpha.AlertEndpoint]
}

// Responsible for anything related to persistent storage
// external to AlertManager
type StorageNode struct {
	*StorageNodeOptions
}

func NewStorageNode(opts ...StorageNodeOption) *StorageNode {
	options := &StorageNodeOptions{
		timeout: time.Second * 60,
	}
	options.storage = future.New[*StorageAPIs]()
	options.apply(opts...)
	if options.Logger == nil {
		options.Logger = logger.NewPluginLogger().Named("alerting-storage-node")
	}
	return &StorageNode{
		options,
	}
}

type StorageNodeOptions struct {
	Logger  *zap.SugaredLogger
	timeout time.Duration
	storage future.Future[*StorageAPIs]
}

type StorageNodeOption func(*StorageNodeOptions)

func WithStorageTimeout(timeout time.Duration) StorageNodeOption {
	return func(o *StorageNodeOptions) {
		o.timeout = timeout
	}
}

func WithLogger(lg *zap.SugaredLogger) StorageNodeOption {
	return func(o *StorageNodeOptions) {
		o.Logger = lg
	}
}

func WithStorage(storage *StorageAPIs) StorageNodeOption {
	return func(o *StorageNodeOptions) {
		o.storage = future.New[*StorageAPIs]()
		o.storage.Set(storage)
	}
}

func (s *StorageNodeOptions) apply(opts ...StorageNodeOption) {
	for _, opt := range opts {
		opt(s)
	}
}

func (s *StorageNode) CreateConditionStorage(ctx context.Context, conditionId string, condition *alertingv1alpha.AlertCondition) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return err
	}
	return storage.Conditions.Put(ctx, path.Join(conditionPrefix, conditionId), condition)
}

func (s *StorageNode) GetConditionStorage(ctx context.Context, conditionId string) (*alertingv1alpha.AlertCondition, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return storage.Conditions.Get(ctx, path.Join(conditionPrefix, conditionId))
}

func (s *StorageNode) ListConditionStorage(ctx context.Context) ([]*alertingv1alpha.AlertCondition, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	items, err := list(ctx, storage.Conditions, conditionPrefix)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *StorageNode) ListWithKeyConditionStorage(ctx context.Context) ([]string, []*alertingv1alpha.AlertCondition, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return nil, nil, err
	}
	keys, items, err := listWithKeys(ctx, storage.Conditions, conditionPrefix)
	if err != nil {
		return nil, nil, err
	}
	return keys, items, nil
}

func (s *StorageNode) UpdateConditionStorage(ctx context.Context, conditionId string, newCondition *alertingv1alpha.AlertCondition) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return err
	}
	_, err = storage.Conditions.Get(ctx, path.Join(conditionPrefix, conditionId))
	if err != nil {
		return shared.WithNotFoundErrorf("condition to update '%s' not found : %s", conditionId, err)
	}
	return storage.Conditions.Put(ctx, path.Join(conditionPrefix, conditionId), newCondition)
}

func (s *StorageNode) DeleteConditionStorage(ctx context.Context, conditionId string) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return err
	}
	_, err = storage.Conditions.Get(ctx, path.Join(conditionPrefix, conditionId))
	if err != nil {
		return shared.WithNotFoundErrorf("condition to delete '%s' not found : %s", conditionId, err)
	}
	return storage.Conditions.Delete(ctx, path.Join(conditionPrefix, conditionId))
}

func (s *StorageNode) CreateEndpointsStorage(ctx context.Context, endpointId string, endpoint *alertingv1alpha.AlertEndpoint) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return err
	}
	return storage.Endpoints.Put(ctx, path.Join(endpointPrefix, endpointId), endpoint)
}

func (s *StorageNode) GetEndpointStorage(ctx context.Context, endpointId string) (*alertingv1alpha.AlertEndpoint, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	return storage.Endpoints.Get(ctx, path.Join(endpointPrefix, endpointId))
}

func (s *StorageNode) ListEndpointStorage(ctx context.Context) ([]*alertingv1alpha.AlertEndpoint, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return nil, err
	}
	items, err := list(ctx, storage.Endpoints, endpointPrefix)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *StorageNode) ListWithKeyEndpointStorage(ctx context.Context) ([]string, []*alertingv1alpha.AlertEndpoint, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return nil, nil, err
	}
	keys, items, err := listWithKeys(ctxTimeout, storage.Endpoints, endpointPrefix)
	if err != nil {
		return nil, nil, err
	}
	return keys, items, nil
}

func (s *StorageNode) UpdateEndpointStorage(ctx context.Context, endpointId string, newEndpoint *alertingv1alpha.AlertEndpoint) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return err
	}
	_, err = storage.Endpoints.Get(ctxTimeout, path.Join(endpointPrefix, endpointId))
	if err != nil {
		return shared.WithNotFoundErrorf("condition to update '%s' not found : %s", endpointId, err)
	}
	return storage.Endpoints.Put(ctx, path.Join(endpointPrefix, endpointId), newEndpoint)
}

func (s *StorageNode) DeleteEndpointStorage(ctx context.Context, endpointId string) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	storage, err := s.storage.GetContext(ctxTimeout)
	if err != nil {
		return err
	}
	_, err = storage.Endpoints.Get(ctx, path.Join(endpointPrefix, endpointId))
	if err != nil {
		return shared.WithNotFoundErrorf("condition to delete '%s' not found : %s", endpointId, err)
	}
	return storage.Endpoints.Delete(ctx, path.Join(endpointPrefix, endpointId))
}
