package gateway

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/versions"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	loggingerrors "github.com/rancher/opni/plugins/logging/pkg/errors"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
)

const defaultOpniVersion = "0.9.2-rc3"

type ClusterStatus int

const (
	ClusterStatusPending ClusterStatus = iota + 1
	ClusterStatusGreen
	ClusterStatusYellow
	ClusterStatusRed
	ClusterStatusError
)

func ClusterStatusDescription(s ClusterStatus) string {
	switch s {
	case ClusterStatusPending:
		return "Opensearch cluster is initializing"
	case ClusterStatusGreen:
		return "Opensearch cluster is green"
	case ClusterStatusYellow:
		return "Opensearch cluster is yellow"
	case ClusterStatusRed:
		return "Opensearch cluster is red"
	case ClusterStatusError:
		return "Error fetching status from Opensearch cluster"
	default:
		return "unknown status"
	}
}

type LoggingManagerV2 struct {
	loggingadmin.UnsafeLoggingAdminV2Server
	managementDriver  management.ClusterDriver
	backendDriver     backend.ClusterDriver
	logger            *zap.SugaredLogger
	opensearchManager *opensearchdata.Manager
	otelForwarder     *otel.OTELForwarder
	storageNamespace  string
	natsRef           *corev1.LocalObjectReference
}

func (m *LoggingManagerV2) GetOpensearchCluster(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.OpensearchClusterV2, error) {
	return m.managementDriver.GetCluster(ctx)
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
		return &emptypb.Empty{}, loggingerrors.ErrInvalidRetention()
	}
	// Input should always have a data nodes field
	if cluster.GetDataNodes() == nil {
		return &emptypb.Empty{}, loggingerrors.ErrMissingDataNode()
	}

	go m.opensearchManager.SetClient(m.managementDriver.NewOpensearchClientForCluster)
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

func (m *LoggingManagerV2) DoUpgrade(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	version := strings.TrimPrefix(versions.Version, "v")
	if version == "unversioned" {
		version = defaultOpniVersion
	}

	err := m.managementDriver.DoUpgrade(ctx, version)

	return &emptypb.Empty{}, err
}

func (m *LoggingManagerV2) GetStorageClasses(ctx context.Context, _ *emptypb.Empty) (*loggingadmin.StorageClassResponse, error) {
	storageClassNames, err := m.managementDriver.GetStorageClasses(ctx)
	if err != nil {
		m.logger.Errorf("failed to list storageclasses: %v", err)
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
	}

	return &loggingadmin.StatusResponse{
		Status:  int32(status),
		Details: ClusterStatusDescription(status),
	}, nil
}

func (m *LoggingManagerV2) validDurationString(duration string) bool {
	match, err := regexp.MatchString(`^\d+[dMmyh]`, duration)
	if err != nil {
		m.logger.Errorf("could not run regexp: %v", err)
		return false
	}
	return match
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
