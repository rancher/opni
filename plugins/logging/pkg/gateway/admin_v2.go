package gateway

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/versions"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/alerting"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	"log/slog"
)

const defaultOpniVersion = "0.11.2"

type ClusterStatus int

const (
	ClusterStatusPending ClusterStatus = iota + 1
	ClusterStatusGreen
	ClusterStatusYellow
	ClusterStatusRed
	ClusterStatusError
)

func ClusterStatusDescription(s ClusterStatus, extraInfo ...string) string {
	switch s {
	case ClusterStatusPending:
		return fmt.Sprintf("Opensearch cluster is initializing. %s", strings.Join(extraInfo, ", "))
	case ClusterStatusGreen:
		return fmt.Sprintf("Opensearch cluster is green. %s", strings.Join(extraInfo, ", "))
	case ClusterStatusYellow:
		return fmt.Sprintf("Opensearch cluster is yellow. %s", strings.Join(extraInfo, ", "))
	case ClusterStatusRed:
		return fmt.Sprintf("Opensearch cluster is red. %s", strings.Join(extraInfo, ", "))
	case ClusterStatusError:
		return fmt.Sprintf("Error fetching status from cluster. %s", strings.Join(extraInfo, ", "))
	default:
		return fmt.Sprintf("Unknown status. %s", strings.Join(extraInfo, ", "))
	}
}

var defaultIndices = []string{
	"logs*",
	"opni-cluster-metadata",
}

type LoggingManagerV2 struct {
	loggingadmin.UnsafeLoggingAdminV2Server
	managementDriver  management.ClusterDriver
	backendDriver     backend.ClusterDriver
	logger            *slog.Logger
	alertingServer    *alerting.AlertingManagementServer
	opensearchManager *opensearchdata.Manager
	otelForwarder     *otel.OTELForwarder
	storageNamespace  string
	natsRef           *corev1.LocalObjectReference
	k8sObjectsName    string
}

func (m *LoggingManagerV2) GetOpensearchCluster(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.OpensearchClusterV2, error) {
	cluster, err := m.managementDriver.GetCluster(ctx)
	if err != nil {
		return nil, err
	}
	return cluster, nil
}

func (m *LoggingManagerV2) DeleteOpensearchCluster(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// Check that it is safe to delete the cluster
	m.opensearchManager.UnsetClient()

	// Remove the state tracking the initial admin
	err := m.opensearchManager.DeleteInitialAdminState()
	if err != nil {
		return nil, err
	}

	err = m.managementDriver.DeleteCluster(ctx)
	if err != nil {
		if errors.Is(err, loggingerrors.ErrLoggingCapabilityExists) {
			m.logger.Error("can not delete opensearch until logging capability is uninstalled from all clusters")
		}
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *LoggingManagerV2) CreateOrUpdateOpensearchCluster(ctx context.Context, cluster *loggingadmin.OpensearchClusterV2) (*emptypb.Empty, error) {
	// Validate retention string
	if !m.validDurationString(lo.FromPtrOr(cluster.DataRetention, "7d")) {
		return &emptypb.Empty{}, loggingerrors.ErrInvalidDuration
	}
	// Input should always have a data nodes field
	if cluster.GetDataNodes() == nil {
		return &emptypb.Empty{}, loggingerrors.ErrMissingDataNode
	}

	// Don't allow a single data node with no persistence
	if err := m.validateStorage(cluster.GetDataNodes()); err != nil {
		return nil, err
	}

	go m.opensearchManager.SetClient(m.managementDriver.NewOpensearchClientForCluster)
	go m.alertingServer.SetClient(m.managementDriver.NewOpensearchClientForCluster)
	m.otelForwarder.BackgroundInitClient()

	version := strings.TrimPrefix(versions.Version, "v")
	if version == "unversioned" {
		version = defaultOpniVersion
	}

	err := m.managementDriver.CreateOrUpdateCluster(ctx, cluster, version, m.natsRef.Name)
	if err != nil {
		return nil, err
	}

	err = m.createInitialAdmin()
	return &emptypb.Empty{}, err
}

func (m *LoggingManagerV2) UpgradeAvailable(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.UpgradeAvailableResponse, error) {
	version := strings.TrimPrefix(versions.Version, "v")
	if version == "unversioned" {
		version = defaultOpniVersion
	}

	upgrade, err := m.managementDriver.UpgradeAvailable(ctx, version)
	if err != nil {
		return nil, err
	}

	return &loggingadmin.UpgradeAvailableResponse{
		UpgradePending: upgrade,
	}, nil
}

func (m *LoggingManagerV2) DoUpgrade(ctx context.Context, options *loggingadmin.UpgradeOptions) (*emptypb.Empty, error) {
	version := strings.TrimPrefix(versions.Version, "v")
	if version == "unversioned" {
		version = defaultOpniVersion
	}

	if options.SnapshotCluster {
		cluster, err := m.managementDriver.GetCluster(ctx)
		if err != nil {
			return nil, err
		}
		if cluster.GetS3() == nil {
			return nil, loggingerrors.ErrInvalidUpgradeOptions
		}

		err = m.opensearchManager.DoSnapshot(ctx, m.k8sObjectsName, defaultIndices)
		if err != nil {
			return nil, err
		}
	}

	err := m.managementDriver.DoUpgrade(ctx, version)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *LoggingManagerV2) GetStorageClasses(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.StorageClassResponse, error) {
	storageClassNames, err := m.managementDriver.GetStorageClasses(ctx)
	if err != nil {
		m.logger.Error(fmt.Sprintf("failed to list storageclasses: %v", err))
		return nil, err
	}

	return &loggingadmin.StorageClassResponse{
		StorageClasses: storageClassNames,
	}, nil
}

func (m *LoggingManagerV2) GetOpensearchStatus(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.StatusResponse, error) {
	status := ClusterStatus(-1)

	installStatus := m.backendDriver.GetInstallStatus(ctx)
	switch installStatus {
	case backend.Absent:
		return nil, grpcstatus.Error(codes.NotFound, "unable to list cluster status")
	case backend.Error:
		status = ClusterStatusError
		return &loggingadmin.StatusResponse{
			Status:  int32(status),
			Details: ClusterStatusDescription(status),
		}, nil
	case backend.Installed:
	default:
		status = ClusterStatusPending
		return &loggingadmin.StatusResponse{
			Status:  int32(status),
			Details: ClusterStatusDescription(status),
		}, nil
	}

	statusResp := m.opensearchManager.GetClusterStatus()
	switch statusResp {
	case opensearchdata.ClusterStatusGreen:
		status = ClusterStatusGreen
	case opensearchdata.ClusterStatusYellow:
		status = ClusterStatusYellow
	case opensearchdata.ClusterStatusRed:
		status = ClusterStatusRed
	case opensearchdata.ClusterStatusError:
		status = ClusterStatusError
	case opensearchdata.ClusterStatusNoClient:
		return &loggingadmin.StatusResponse{
			Status:  int32(ClusterStatusPending),
			Details: ClusterStatusDescription(ClusterStatusPending, "Waiting for Opensearch client to be initialized"),
		}, nil
	}

	return &loggingadmin.StatusResponse{
		Status:  int32(status),
		Details: ClusterStatusDescription(status),
	}, nil
}

func (m *LoggingManagerV2) CreateOrUpdateSnapshotSchedule(
	ctx context.Context,
	snapshot *loggingadmin.SnapshotSchedule,
) (*emptypb.Empty, error) {
	if snapshot.GetRetention().GetTimeRetention() != "" &&
		!m.validDurationString(snapshot.GetRetention().GetTimeRetention()) {
		return nil, loggingerrors.ErrInvalidDuration
	}

	err := m.managementDriver.CreateOrUpdateSnapshotSchedule(ctx, snapshot, defaultIndices)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (m *LoggingManagerV2) GetSnapshotSchedule(
	ctx context.Context,
	ref *loggingadmin.SnapshotReference,
) (*loggingadmin.SnapshotSchedule, error) {
	snapshot, err := m.managementDriver.GetSnapshotSchedule(ctx, ref, defaultIndices)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (m *LoggingManagerV2) DeleteSnapshotSchedule(ctx context.Context, ref *loggingadmin.SnapshotReference) (*emptypb.Empty, error) {
	err := m.managementDriver.DeleteSnapshotSchedule(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (m *LoggingManagerV2) ListSnapshotSchedules(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.SnapshotStatusList, error) {
	list, err := m.managementDriver.ListAllSnapshotSchedules(ctx)
	if err != nil {
		return nil, err
	}

	return list, nil
}

func (m *LoggingManagerV2) validDurationString(duration string) bool {
	match, err := regexp.MatchString(`^\d+[dMmyh]`, duration)
	if err != nil {
		m.logger.Error(fmt.Sprintf("could not run regexp: %v", err))
		return false
	}
	return match
}

func (m *LoggingManagerV2) validateStorage(dataNodes *loggingadmin.DataDetails) error {
	if dataNodes.GetReplicas() < 2 && !dataNodes.GetPersistence().GetEnabled() {
		m.logger.Error("minimum of 2 data nodes required if no persistent storage")
		return loggingerrors.ErrInvalidDataPersistence
	}
	return nil
}

func (m *LoggingManagerV2) opensearchClusterReady() bool {
	absentRetriesMax := 3
	ctx := context.TODO()
	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(5*time.Second),
		backoff.WithMaxInterval(1*time.Minute),
		backoff.WithMultiplier(1.1),
	)
	b := expBackoff.Start(ctx)

FETCH:
	for {
		absentRetries := 0
		select {
		case <-b.Done():
			m.logger.Warn("plugin context cancelled before Opensearch object created")
			return true
		case <-b.Next():
			state := m.backendDriver.GetInstallStatus(ctx)
			switch state {
			case backend.Error:
				m.logger.Error("failed to fetch opensearch cluster, can't check readiness")
				return true
			case backend.Absent:
				absentRetries++
				if absentRetries > absentRetriesMax {
					m.logger.Error("failed to fetch opensearch cluster, can't check readiness")
					return true
				}
				continue
			case backend.Pending:
				continue
			case backend.Installed:
				break FETCH
			}
		}
	}
	return false
}

func (m *LoggingManagerV2) createInitialAdmin() error {
	password, err := m.managementDriver.AdminPassword(context.TODO())
	if err != nil {
		return err
	}

	go m.opensearchManager.CreateInitialAdmin(password, m.opensearchClusterReady)
	return nil
}
