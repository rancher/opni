package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/rancher/opni/pkg/util"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/opts"
	storage_opts "github.com/rancher/opni/pkg/alerting/storage/opts"
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

func (c *CompositeAlertingClientSet) GetHash(_ context.Context, key string) string {
	if _, ok := c.hashes[key]; !ok {
		return ""
	}
	return c.hashes[key]
}

func (c *CompositeAlertingClientSet) CalculateHash(ctx context.Context, key string, syncOptions *storage_opts.SyncOptions) error {
	aggregate := ""
	if syncOptions.DefaultEndpoint != "" {
		aggregate += syncOptions.DefaultEndpoint
	}
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

func (c *CompositeAlertingClientSet) calculateRouters(ctx context.Context, syncOpts *storage_opts.SyncOptions) ([]string, error) {
	key := shared.SingleConfigId
	// List all conditions & map their endpoints
	conds, err := c.Conditions().List(ctx)
	if err != nil {
		return nil, err
	}
	endps, err := c.Endpoints().List(ctx, opts.WithUnredacted())
	if err != nil {
		return nil, err
	}

	endpMap := lo.Associate(endps, func(a *alertingv1.AlertEndpoint) (string, *alertingv1.AlertEndpoint) {
		return a.Id, a
	})

	if syncOpts.Router == nil {
		syncOpts.Router = routing.NewDefaultOpniRouting()
	}
	// create router specs for conditions
	for _, cond := range conds {
		if cond.Id == "" {
			cond.Id = cond.GetClusterId().GetId()
		}
		ns := cond.Namespace()
		endpoints := cond.GetAttachedEndpoints()
		if endpoints == nil || len(endpoints.Items) == 0 {
			// this registers it with the virtual router, but doesn't ever build it into the physical router
			err = syncOpts.Router.SetNamespaceSpec(ns, cond.Id, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{},
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		endpointConfigs := make([]*alertingv1.AlertEndpoint, len(endpoints.GetItems()))
		for i, endpoint := range endpoints.GetItems() {
			i := i
			endpoint := endpoint
			if endp, ok := endpMap[endpoint.EndpointId]; ok {
				endpointConfigs[i] = endp
			} else {
				return nil, status.Error(codes.NotFound, "endpoint not found")
			}
		}
		routingNode, err := backend.ConvertEndpointIdsToRoutingNode(
			endpointConfigs,
			endpoints,
			cond.Id,
		)
		if err != nil {
			return nil, err
		}
		err = syncOpts.Router.SetNamespaceSpec(ns, cond.Id, routingNode.GetFullAttachedEndpoints())
		if err != nil {
			return nil, err
		}
		if util.StatusCode(err) == codes.InvalidArgument { // sync & build logic are mismatched
			panic(err)
		}
	}
	// set expected defaults based on endpoint configuration
	defaults := lo.Filter(endps, func(a *alertingv1.AlertEndpoint, _ int) bool {
		if len(a.GetProperties()) == 0 {
			return false
		}
		_, ok := a.GetProperties()[alertingv1.EndpointTagNotifications]
		return ok
	})
	if err := syncOpts.Router.SetDefaultNamespaceConfig(defaults); err != nil {
		return nil, err
	}
	if util.StatusCode(err) == codes.InvalidArgument { // sync & build logic are mismatched
		panic(err)
	}

	// when we implement attaching endpoints to the default namespace. do this here
	if syncOpts.DefaultEndpoint != "" {
		syncOpts.Router.SetDefaultReceiver(syncOpts.DefaultEndpoint)
	}
	if err := c.Routers().Put(ctx, key, syncOpts.Router); err != nil {
		return nil, err
	}
	return []string{key}, nil
}

// Based on the other storage, calculate what the virtual config should be,
// then overwrite the virtual config storage
func (c *CompositeAlertingClientSet) ForceSync(ctx context.Context, incomingOpts ...storage_opts.SyncOption) error {
	syncOpts := storage_opts.NewSyncOptions()
	syncOpts.Apply(incomingOpts...)
	if err := c.CalculateHash(ctx, shared.SingleConfigId, syncOpts); err != nil {
		return err
	}
	c.Logger.With("hash", c.GetHash(ctx, shared.SingleConfigId)).Debug("starting force sync")

	_, err := c.calculateRouters(ctx, syncOpts)
	c.Logger.Debug("finished force sync")
	return err
}

func (c *CompositeAlertingClientSet) Sync(ctx context.Context, incomingOpts ...storage_opts.SyncOption) ([]string, error) {
	syncOpts := storage_opts.NewSyncOptions()
	syncOpts.Apply(incomingOpts...)
	key := shared.SingleConfigId
	curHash := strings.Clone(c.GetHash(ctx, key))
	c.CalculateHash(ctx, key, syncOpts)
	newHash := c.GetHash(ctx, key)
	if curHash == newHash {
		return []string{}, nil
	}
	keys, err := c.calculateRouters(ctx, syncOpts)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (c *CompositeAlertingClientSet) Purge(ctx context.Context) error {
	errG, ctxCa := errgroup.WithContext(ctx)
	errG.Go(func() error {
		keys, err := c.Conditions().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := c.Conditions().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := c.Endpoints().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := c.Endpoints().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := c.Routers().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := c.Routers().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := c.States().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := c.States().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	errG.Go(func() error {
		keys, err := c.Incidents().ListKeys(ctxCa)
		if err != nil {
			return err
		}
		for _, key := range keys {
			err := c.Incidents().Delete(ctxCa, key)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return errG.Wait()
}
