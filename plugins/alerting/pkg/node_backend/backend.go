package node_backend

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/google/go-cmp/cmp"
	"github.com/rancher/opni/pkg/agent"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/drivers"
)

type CapabilitySpecKV struct {
	DefaultCapabilitySpec storage.ValueStoreT[*node.AlertingCapabilitySpec]
	NodeCapabilitySpecs   storage.KeyValueStoreT[*node.AlertingCapabilitySpec]
}

type AlertingNodeBackend struct {
	util.Initializer

	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeAlertingCapabilityServer
	node.UnsafeAlertingNodeConfigurationServer

	lg *zap.SugaredLogger

	nodeStatusMu sync.RWMutex
	nodeStatus   map[string]*capabilityv1.NodeCapabilityStatus

	delegate       future.Future[streamext.StreamDelegate[agent.ClientSet]]
	mgmtClient     future.Future[managementv1.ManagementClient]
	storageBackend future.Future[storage.Backend]

	capabilityKV future.Future[CapabilitySpecKV]
}

func NewAlertingNodeBackend(
	lg *zap.SugaredLogger,
) *AlertingNodeBackend {
	return &AlertingNodeBackend{
		lg:             lg,
		delegate:       future.New[streamext.StreamDelegate[agent.ClientSet]](),
		mgmtClient:     future.New[managementv1.ManagementClient](),
		storageBackend: future.New[storage.Backend](),
		capabilityKV:   future.New[CapabilitySpecKV](),
		nodeStatus:     make(map[string]*capabilityv1.NodeCapabilityStatus),
	}
}

func (a *AlertingNodeBackend) Initialize(
	kv CapabilitySpecKV,
	mgmtClient managementv1.ManagementClient,
	nodeManagerClient streamext.StreamDelegate[agent.ClientSet],
	storageBackend storage.Backend,
) {
	a.InitOnce(func() {
		a.capabilityKV.Set(kv)
		a.delegate.Set(nodeManagerClient)
		a.mgmtClient.Set(mgmtClient)
		a.storageBackend.Set(storageBackend)
	})
}

var (
	_ node.NodeAlertingCapabilityServer    = (*AlertingNodeBackend)(nil)
	_ node.AlertingNodeConfigurationServer = (*AlertingNodeBackend)(nil)
	_ capabilityv1.BackendServer           = (*AlertingNodeBackend)(nil)
)

var (
	// The "default" default node spec. Exported for testing purposes.
	FallbackDefaultNodeSpec atomic.Pointer[node.AlertingCapabilitySpec]
)

func init() {
	FallbackDefaultNodeSpec.Store(
		&node.AlertingCapabilitySpec{
			RuleDiscovery: &node.RuleDiscoverySpec{
				Enabled: true,
			},
		},
	)
}

func (a *AlertingNodeBackend) requestNodeSync(ctx context.Context, target *corev1.Reference) error {
	if target == nil || target.Id == "" {
		panic("bug: target must be non-nil and have a non-empty ID. this logic was recently changed - please update the caller")
	}
	_, err := a.delegate.Get().
		WithTarget(target).
		SyncNow(ctx, &capabilityv1.Filter{CapabilityNames: []string{wellknown.CapabilityAlerting}})
	return err
}

func (a *AlertingNodeBackend) broadcastNodeSync(ctx context.Context) {
	// keep any metadata in the context, but don't propagate cancellation
	ctx = context.WithoutCancel(ctx)
	var errs []error
	a.delegate.Get().
		WithBroadcastSelector(&corev1.ClusterSelector{}, func(reply any, msg *streamv1.BroadcastReplyList) error {
			for _, resp := range msg.GetResponses() {
				err := resp.GetReply().GetResponse().GetStatus().Err()
				if err != nil {
					target := resp.GetRef()
					errs = append(errs, status.Errorf(codes.Internal, "failed to sync agent %s: %v", target.GetId(), err))
				}
			}
			return nil
		}).
		SyncNow(ctx, &capabilityv1.Filter{
			CapabilityNames: []string{wellknown.CapabilityAlerting},
		})
	if len(errs) > 0 {
		a.lg.With(
			zap.Error(errors.Join(errs...)),
		).Warn("one or more agents failed to sync; they may not be updated immediately")
	}
}

func (a *AlertingNodeBackend) buildResponse(oldCfg, newCfg *node.AlertingCapabilityConfig) *node.SyncResponse {
	oldIgnoreConditions := util.ProtoClone(oldCfg)
	if oldIgnoreConditions != nil {
		oldIgnoreConditions.Conditions = nil
	}
	newIgnoreConditions := util.ProtoClone(newCfg)
	if newIgnoreConditions != nil {
		newIgnoreConditions.Conditions = nil
	}
	if cmp.Equal(oldIgnoreConditions, newIgnoreConditions, protocmp.Transform()) {
		return &node.SyncResponse{
			ConfigStatus: corev1.ConfigStatus_UpToDate,
		}
	}
	return &node.SyncResponse{
		ConfigStatus:  corev1.ConfigStatus_NeedsUpdate,
		UpdatedConfig: newCfg,
	}
}

func (a *AlertingNodeBackend) GetDefaultConfiguration(ctx context.Context, _ *emptypb.Empty) (*node.AlertingCapabilitySpec, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting Node Backend is not yet initialized")
	}
	return a.getDefaultNodeSpec(ctx)
}

func (a *AlertingNodeBackend) SetDefaultConfiguration(ctx context.Context, spec *node.AlertingCapabilitySpec) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting Node Backend is not yet initialized")
	}
	var empty node.AlertingCapabilitySpec
	if cmp.Equal(spec, &empty, protocmp.Transform()) {
		if err := a.capabilityKV.Get().DefaultCapabilitySpec.Delete(ctx); err != nil {
			return nil, err
		}
		a.broadcastNodeSync(ctx)
		return &emptypb.Empty{}, nil
	}

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	if err := a.capabilityKV.Get().DefaultCapabilitySpec.Put(ctx, spec); err != nil {
		return nil, err
	}

	a.broadcastNodeSync(ctx)

	return &emptypb.Empty{}, nil
}

func (a *AlertingNodeBackend) GetNodeConfiguration(ctx context.Context, node *corev1.Reference) (*node.AlertingCapabilitySpec, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting Node Backend is not yet initialized")
	}
	return a.getNodeSpecOrDefault(ctx, node.Id)
}

func (a *AlertingNodeBackend) SetNodeConfiguration(ctx context.Context, req *node.NodeConfigRequest) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting Node Backend is not yet initialized")
	}

	if req.Spec == nil {
		if err := a.capabilityKV.Get().NodeCapabilitySpecs.Delete(ctx, req.Node.GetId()); err != nil {
			return nil, err
		}
		a.requestNodeSync(ctx, req.GetNode())
		return &emptypb.Empty{}, nil
	}

	if err := req.GetSpec().Validate(); err != nil {
		return nil, err
	}

	if err := a.capabilityKV.Get().NodeCapabilitySpecs.Put(ctx, req.Node.GetId(), req.GetSpec()); err != nil {
		return nil, err
	}
	a.requestNodeSync(ctx, req.GetNode())
	return &emptypb.Empty{}, nil
}

func (a *AlertingNodeBackend) getDefaultNodeSpec(ctx context.Context) (*node.AlertingCapabilitySpec, error) {
	spec, err := a.capabilityKV.Get().DefaultCapabilitySpec.Get(ctx)
	if status.Code(err) == codes.NotFound {
		spec = FallbackDefaultNodeSpec.Load()
	} else if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to get default capability spec : %s", err)
	}
	grpc.SetTrailer(ctx, node.DefaultConfigMetadata())
	return spec, nil
}

func (a *AlertingNodeBackend) getNodeSpecOrDefault(ctx context.Context, id string) (*node.AlertingCapabilitySpec, error) {
	nodeSpec, err := a.capabilityKV.Get().NodeCapabilitySpecs.Get(ctx, id)
	if status.Code(err) == codes.NotFound {
		return a.getDefaultNodeSpec(ctx)
	} else if err != nil {
		a.lg.With(zap.Error(err)).Error("failed to get node capability spec")
		return nil, status.Errorf(codes.Unavailable, "failed to get node capability spec: %v", err)
	}
	return nodeSpec, nil
}

func (a *AlertingNodeBackend) Sync(ctx context.Context, req *node.AlertingCapabilityConfig) (*node.SyncResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting Node Backend is not yet initialized")
	}
	id := cluster.StreamAuthorizedID(ctx)

	cluster, err := a.mgmtClient.Get().GetCluster(ctx, &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, err
	}
	enabled := false
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityAlerting {
			enabled = true
			break
		}
	}
	conditions := []string{}
	a.nodeStatusMu.Lock()
	defer a.nodeStatusMu.Unlock()
	status := a.nodeStatus[id]
	if status == nil {
		a.nodeStatus[id] = &capabilityv1.NodeCapabilityStatus{}
		status = a.nodeStatus[id]
	}

	status.Enabled = req.GetEnabled()
	status.Conditions = req.GetConditions()
	status.LastSync = timestamppb.Now()

	a.lg.With(
		"id", id,
		"time", status.LastSync.AsTime(),
	).Debug("synced node")

	nodeSpec, err := a.getNodeSpecOrDefault(ctx, id)
	if err != nil {
		return nil, err
	}

	return a.buildResponse(req, &node.AlertingCapabilityConfig{
		Enabled:    enabled,
		Conditions: conditions,
		Spec:       nodeSpec,
	}), nil
}

// Returns info about the backend, including capability name
func (a *AlertingNodeBackend) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityAlerting,
		Source:  "plugin_alerting",
		Drivers: drivers.Drivers.List(),
	}, nil
}

// Deprecated: Do not use.
// Returns an error if installing the capability would fail.
func (a *AlertingNodeBackend) CanInstall(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "alerting node backend is not yet initialized")
	}
	return &emptypb.Empty{}, nil
}

// Installs the capability on a cluster.
func (a *AlertingNodeBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting node backend is not yet available")
	}

	var warningErr error
	_, err := a.CanInstall(ctx, &emptypb.Empty{})
	if err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		}
		warningErr = err
	}

	_, err = a.storageBackend.Get().UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityAlerting)),
	)
	if err != nil {
		return nil, err
	}

	a.requestNodeSync(ctx, req.Cluster)

	if warningErr != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Warning,
			Message: warningErr.Error(),
		}, nil
	}
	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

// Returns common runtime config info for this capability from a specific
// cluster (node).
func (a *AlertingNodeBackend) Status(_ context.Context, req *corev1.Reference) (*capabilityv1.NodeCapabilityStatus, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting node backend is not yet available")
	}

	a.nodeStatusMu.RLock()
	defer a.nodeStatusMu.RUnlock()

	if status, ok := a.nodeStatus[req.Id]; ok {
		return util.ProtoClone(status), nil
	}
	return nil, status.Error(codes.NotFound, "no status has been reported for this node")
}

// Requests the backend to clean up any resources it owns and prepare
// for uninstallation. This process is asynchronous. The status of the
// operation can be queried using the UninstallStatus method, or canceled
// using the CancelUninstall method.
func (a *AlertingNodeBackend) Uninstall(ctx context.Context, req *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alerting node backend is not yet available")
	}

	cluster, err := a.mgmtClient.Get().GetCluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	exists := false
	for _, cap := range cluster.GetCapabilities() {
		if cap.Name == wellknown.CapabilityAlerting {
			exists = true
			break
		}
	}
	if !exists {
		return nil, status.Error(codes.FailedPrecondition, "cluster does not have the request capability")
	}

	_, err = a.storageBackend.Get().UpdateCluster(
		ctx,
		req.GetCluster(),
		storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster(wellknown.CapabilityAlerting)),
	)
	if err != nil {
		return nil, err
	}

	a.requestNodeSync(ctx, req.Cluster)
	return &emptypb.Empty{}, nil
}

// Gets the status of the uninstall task for the given cluster.
func (a *AlertingNodeBackend) UninstallStatus(_ context.Context, _ *corev1.Reference) (*corev1.TaskStatus, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability uninstall is not asynchronous")
}

// Cancels an uninstall task for the given cluster, if it is still pending.
func (a *AlertingNodeBackend) CancelUninstall(_ context.Context, _ *corev1.Reference) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability uninstall is not asynchronous")
}

// Deprecated: Do not use.
// Returns a go template string which will generate a shell command used to
// install the capability. This will be displayed to the user in the UI.
// See InstallerTemplateSpec above for the available template fields.
func (a *AlertingNodeBackend) InstallerTemplate(_ context.Context, _ *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Deprecated API: Do not use")
}
