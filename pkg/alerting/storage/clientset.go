package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"slices"

	"github.com/rancher/opni/pkg/util"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	storage_opts "github.com/rancher/opni/pkg/alerting/storage/opts"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
	"log/slog"
)

const defaultTrackerTTL = 24 * time.Hour

type CompositeAlertingClientSet struct {
	conds     spec.ConditionStorage
	endps     spec.EndpointStorage
	routers   spec.RouterStorage
	states    spec.StateStorage
	incidents spec.IncidentStorage
	hashes    map[string]string
	Logger    *slog.Logger
}

var _ spec.AlertingClientSet = (*CompositeAlertingClientSet)(nil)

func (c CompositeAlertingClientSet) Conditions() spec.ConditionStorage {
	return c.conds
}

func (c CompositeAlertingClientSet) Endpoints() spec.EndpointStorage {
	return c.endps
}

func (c CompositeAlertingClientSet) Routers() spec.RouterStorage {
	return c.routers
}

func (c CompositeAlertingClientSet) States() spec.StateStorage {
	return c.states
}

func (c CompositeAlertingClientSet) Incidents() spec.IncidentStorage {
	return c.incidents
}

func (c *CompositeAlertingClientSet) GetHash(ctx context.Context, key string) (string, error) {
	router, err := c.Routers().Get(ctx, key)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			if st.Code() == codes.NotFound {
				return "", nil
			}
		}
		return "", err
	}

	cfg, err := router.BuildConfig()
	if err != nil {
		return "", err
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write(out)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (c *CompositeAlertingClientSet) listAllConditions(ctx context.Context) ([]*alertingv1.AlertCondition, error) {
	groups, err := c.Conditions().ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	conds := []*alertingv1.AlertCondition{}
	for _, groupId := range groups {
		groupConds, err := c.Conditions().Group(groupId).List(ctx)
		if err != nil {
			return nil, err
		}
		conds = append(conds, groupConds...)
	}
	return conds, nil
}

func (c *CompositeAlertingClientSet) calculateRouters(ctx context.Context, syncOpts *storage_opts.SyncOptions) ([]string, error) {
	key := shared.SingleConfigId
	// List all conditions & map their endpoints
	conds, err := c.listAllConditions(ctx)
	if err != nil {
		return nil, err
	}
	endps, err := c.Endpoints().List(ctx, storage_opts.WithUnredacted())
	if err != nil {
		return nil, err
	}

	endpMap := lo.Associate(endps, func(a *alertingv1.AlertEndpoint) (string, *alertingv1.AlertEndpoint) {
		return a.Id, a
	})

	if syncOpts.Router == nil {
		syncOpts.Router = routing.NewDefaultOpniRouting()
	}

	endpKeys := lo.Keys(endpMap)
	slices.Sort(endpKeys)

	for _, endpId := range endpKeys {
		endp := endpMap[endpId]
		err = syncOpts.Router.SetNamespaceSpec(message.TestNamespace, endp.Id, &alertingv1.FullAttachedEndpoints{
			InitialDelay:       durationpb.New(time.Second),
			RepeatInterval:     durationpb.New(time.Hour),
			ThrottlingDuration: durationpb.New(time.Minute),
			Details: &alertingv1.EndpointImplementation{
				Title: "Test admin notification",
				Body:  fmt.Sprintf("Test admin notification : %s", endp.Name),
			},
			Items: []*alertingv1.FullAttachedEndpoint{
				{
					EndpointId:    endp.Id,
					AlertEndpoint: endp,
					Details: &alertingv1.EndpointImplementation{
						Title: "Test admin notification",
						Body:  fmt.Sprintf("Test admin notification : %s", endp.Name),
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}
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
	if syncOpts.DefaultReceiver != nil {
		syncOpts.Router.SetDefaultReceiver(*syncOpts.DefaultReceiver)
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

	_, err := c.calculateRouters(ctx, syncOpts)
	return err
}

func (c *CompositeAlertingClientSet) Sync(ctx context.Context, incomingOpts ...storage_opts.SyncOption) ([]string, error) {
	syncOpts := storage_opts.NewSyncOptions()
	syncOpts.Apply(incomingOpts...)
	keys, err := c.calculateRouters(ctx, syncOpts)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (c *CompositeAlertingClientSet) Purge(ctx context.Context) error {
	errG, ctxCa := errgroup.WithContext(ctx)
	errG.Go(func() error {
		groups, err := c.Conditions().ListGroups(ctxCa)
		if err != nil {
			return err
		}
		for _, groupId := range groups {
			keys, err := c.Conditions().Group(groupId).ListKeys(ctxCa)
			if err != nil {
				return err
			}
			for _, key := range keys {
				err := c.Conditions().Group(groupId).Delete(ctxCa, key)
				if err != nil {
					return err
				}
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
