package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/storage_opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

const defaultTrackerTTL = 24 * time.Hour

type CompositeAlertingClientSet struct {
	conds     ConditionStorage
	endps     EndpointStorage
	routers   RouterStorage
	states    StateStorage
	incidents IncidentStorage
	hashes    map[string]string
	Logger    *zap.SugaredLogger
}

var _ AlertingClientSet = (*CompositeAlertingClientSet)(nil)

func (c CompositeAlertingClientSet) Conditions() ConditionStorage {
	return c.conds
}

func (c CompositeAlertingClientSet) Endpoints() EndpointStorage {
	return c.endps
}

func (c CompositeAlertingClientSet) Routers() RouterStorage {
	return c.routers
}

func (c CompositeAlertingClientSet) States() StateStorage {
	return c.states
}

func (c CompositeAlertingClientSet) Incidents() IncidentStorage {
	return c.incidents
}

func (c *CompositeAlertingClientSet) resolveHashKey(key string) {
	if key == shared.SingleConfigId {
		// store all cluster configurations in a single virtual config
	} else {
		// multi-cluster multi tenant virutal configs
		panic("not implemented")
	}
}

func (c *CompositeAlertingClientSet) GetHash(ctx context.Context, key string) string {
	c.resolveHashKey(key)
	if _, ok := c.hashes[key]; !ok {
		return ""
	}
	return c.hashes[key]
}

// TODO : better implementation
func (c *CompositeAlertingClientSet) CalculateHash(ctx context.Context, key string) error {
	aggregate := ""
	if key == shared.SingleConfigId {
		conds, err := c.Conditions().List(ctx)
		if err != nil {
			return err
		}
		aggregate += strings.Join(
			lo.Map(conds, func(a *alertingv1.AlertCondition, _ int) string {
				return a.Id + a.LastUpdated.String()
			}), "-")
		endps, err := c.Endpoints().List(ctx)
		if err != nil {
			return err
		}
		aggregate += strings.Join(
			lo.Map(endps, func(a *alertingv1.AlertEndpoint, _ int) string {
				return a.Id + a.LastUpdated.String()
			}), "_")
	} else {
		panic("not implemented")
	}
	encode := strings.NewReader(aggregate)
	hash := sha256.New()
	if _, err := io.Copy(hash, encode); err != nil {
		return err
	}
	c.hashes[key] = hex.EncodeToString(hash.Sum(nil))
	return nil
}

// Based on the other storage, calculate what the virtual config should be,
// then overwrite the virtual config storage
// ! only returns an error if the overwrite fails
// TODO : better implementation
func (c *CompositeAlertingClientSet) ForceSync(ctx context.Context, opts ...storage_opts.SyncOption) error {
	syncOpts := storage_opts.NewSyncOptions()
	syncOpts.Apply(opts...)
	if err := c.CalculateHash(ctx, shared.SingleConfigId); err != nil {
		return err
	}
	c.Logger.With("hash", c.GetHash(ctx, shared.SingleConfigId)).Debug("starting force sync")

	key := shared.SingleConfigId

	// List all conditions & map their endpoints
	conds, err := c.Conditions().List(ctx)
	if err != nil {
		return err
	}
	// conditionId -> endpointId
	relationMap := make(map[string]*alertingv1.AttachedEndpoints)
	for _, c := range conds {
		if c.Id == "" {
			c.Id = c.GetClusterId().GetId()

		}
		//FIXME: hack
		ns := "prometheusQuery"
		if c.GetAttachedEndpoints() != nil {
			relationMap[ns+"-"+c.Id] = c.GetAttachedEndpoints()
		}
	}
	// get unredacted endpoint details
	for encodedNsId, endpoints := range relationMap {
		namespace, id := strings.Split(encodedNsId, "-")[0], strings.Split(encodedNsId, "-")[1] //FIXME:
		endpointConfigs := make([]*alertingv1.AlertEndpoint, len(endpoints.GetItems()))
		var errG errgroup.Group
		for i, endpoint := range endpoints.GetItems() {
			i := i
			endpoint := endpoint
			errG.Go(func() error {
				endpoint, err := c.Endpoints().Get(ctx, endpoint.EndpointId, storage_opts.WithUnredacted())
				if err != nil {
					return err
				}
				endpointConfigs[i] = endpoint
				return err
			})

		}
		err := errG.Wait()
		if err != nil {
			return err
		}
		routingNode, err := backend.ConvertEndpointIdsToRoutingNode(
			endpointConfigs,
			endpoints,
			id,
		)
		if err != nil {
			return err
		}
		err = syncOpts.Router.SetNamespaceSpec(namespace, id, routingNode.GetFullAttachedEndpoints())
		if err != nil {
			return err
		}

	}
	// when we implement attaching endpoints to the default namespace. do this here

	err = c.Routers().Put(ctx, key, syncOpts.Router)
	c.Logger.Debug("finished force sync")
	return err
}

func (c *CompositeAlertingClientSet) Sync(ctx context.Context, opts ...storage_opts.SyncOption) ([]string, error) {
	syncOpts := storage_opts.NewSyncOptions()
	syncOpts.Apply(opts...)

	key := shared.SingleConfigId

	curHash := strings.Clone(c.GetHash(ctx, key))
	c.CalculateHash(ctx, key)
	newHash := c.GetHash(ctx, key)
	if curHash == newHash {
		return []string{}, nil
	}

	// conditionId -> endpointId
	relationMap := make(map[string]*alertingv1.AttachedEndpoints)
	// List all conditions & map their endpoints
	conds, err := c.Conditions().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range conds {
		if c.Id == "" {
			c.Id = c.GetClusterId().GetId()

		}
		//FIXME: hack
		ns := "prometheusQuery"
		if c.GetAttachedEndpoints() != nil {
			relationMap[ns+"-"+c.Id] = c.GetAttachedEndpoints()
		}
	}
	// get unredacted endpoint details
	for encodedNsId, endpoints := range relationMap {
		namespace, id := strings.Split(encodedNsId, "-")[0], strings.Split(encodedNsId, "-")[1] //FIXME:
		endpointConfigs := make([]*alertingv1.AlertEndpoint, len(endpoints.GetItems()))
		var errG errgroup.Group
		for i, endpoint := range endpoints.GetItems() {
			i := i
			endpoint := endpoint
			errG.Go(func() error {
				endpoint, err := c.Endpoints().Get(ctx, endpoint.EndpointId, storage_opts.WithUnredacted())
				if err != nil {
					return err
				}
				endpointConfigs[i] = endpoint
				return err
			})

		}
		err := errG.Wait()
		if err != nil {
			return nil, err
		}
		routingNode, err := backend.ConvertEndpointIdsToRoutingNode(
			endpointConfigs,
			endpoints,
			id,
		)
		if err != nil {
			return nil, err
		}
		err = syncOpts.Router.SetNamespaceSpec(namespace, id, routingNode.GetFullAttachedEndpoints())
		if err != nil {
			return nil, err
		}
	}
	// when we implement attaching endpoints to the default namespace. do this here

	err = c.Routers().Put(ctx, key, syncOpts.Router)
	return []string{key}, err
}

func (s *CompositeAlertingClientSet) Purge(ctx context.Context) error {
	errG, ctxCa := errgroup.WithContext(ctx)
	errG.Go(func() error {
		keys, err := s.Conditions().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := s.Conditions().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := s.Endpoints().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := s.Endpoints().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := s.Routers().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := s.Routers().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := s.States().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := s.States().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := s.Incidents().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := s.Incidents().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return errG.Wait()
}
