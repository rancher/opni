package slo

import (
	"context"
	"fmt"
	"path"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func list[T proto.Message](ctx context.Context, kvc storage.KeyValueStoreT[T], prefix string) ([]T, error) {
	keys, err := kvc.ListKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

func checkDatasource(datasource string) error {
	datasourceMu.RLock()
	defer datasourceMu.RUnlock()
	if _, ok := datasources[datasource]; !ok {
		return shared.ErrInvalidDatasource
	}
	return nil
}

func (s *SLOServerComponent) GetSLO(ctx context.Context, ref *corev1.Reference) (*slov1.SLOData, error) {
	return s.storage.Get().SLOs.Get(ctx, path.Join("/slos", ref.Id))
}

func (s *SLOServerComponent) ListSLOs(ctx context.Context, _ *emptypb.Empty) (*slov1.ServiceLevelObjectiveList, error) {
	items, err := list(ctx, s.storage.Get().SLOs, "/slos")
	if err != nil {
		return nil, err
	}
	return &slov1.ServiceLevelObjectiveList{
		Items: items,
	}, nil
}

func (s *SLOServerComponent) CreateSLO(ctx context.Context, req *slov1.CreateSLORequest) (*corev1.Reference, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	clusterList, err := s.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	if !clusterList.Has(req.Slo.ClusterId) {
		return nil, validation.Error("invalid cluster : not found")
	}
	datasourceMu.RLock()
	sloStore := datasources[req.GetSlo().GetDatasource()]
	datasourceMu.RUnlock()

	if err := sloStore.Precondition(ctx, &corev1.Reference{Id: req.Slo.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}

	id, err := sloStore.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	sloData := &slov1.SLOData{
		Id:        id.Id,
		SLO:       req.GetSlo(),
		CreatedAt: timestamppb.New(time.Now()),
	}
	if err := s.storage.Get().SLOs.Put(ctx, path.Join("/slos", id.Id), sloData); err != nil {
		return nil, err
	}
	return id, nil
}

func (s *SLOServerComponent) UpdateSLO(ctx context.Context, req *slov1.SLOData) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	clusterList, err := s.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	validCluster := false
	for _, clusterListItem := range clusterList.Items {
		if clusterListItem.Id == req.SLO.ClusterId {
			validCluster = true
			break
		}
	}
	if !validCluster {
		return nil, validation.Error("invalid cluster")
	}
	existing, err := s.storage.Get().SLOs.Get(ctx, path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}
	sloStore := datasources[req.GetSLO().GetDatasource()]
	if err := sloStore.Precondition(ctx, &corev1.Reference{Id: req.SLO.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	updatedSLO, err := sloStore.Update(ctx, req, existing)
	if err != nil { // exit when update fails
		return nil, err
	}
	// Merge when everything else is done
	if err := s.storage.Get().SLOs.Put(ctx, path.Join("/slos", req.Id), updatedSLO); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *SLOServerComponent) DeleteSLO(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	lg := s.logger
	existing, err := s.storage.Get().SLOs.Get(ctx, path.Join("/slos", req.Id))
	if err != nil {
		lg.With("delete slo", req.Id).Error("failed to get slo to delete in K,V store")
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasources[existing.SLO.GetDatasource()]
	if err := sloStore.Precondition(ctx, &corev1.Reference{Id: existing.SLO.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	err = sloStore.Delete(ctx, existing)
	if err != nil {
		return nil, err
	}
	if err := s.storage.Get().SLOs.Delete(ctx, path.Join("/slos", req.Id)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *SLOServerComponent) CloneSLO(ctx context.Context, ref *corev1.Reference) (*slov1.SLOData, error) {
	existing, err := s.storage.Get().SLOs.Get(ctx, path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}
	if err := existing.Validate(); err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasources[existing.SLO.GetDatasource()]
	if err := sloStore.Precondition(ctx, &corev1.Reference{Id: existing.SLO.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	newId, newData, anyError := sloStore.Clone(ctx, existing)
	newData.CreatedAt = timestamppb.New(time.Now())
	if anyError != nil {
		return nil, anyError
	}
	if err := s.storage.Get().SLOs.Put(ctx, path.Join("/slos", newId.Id), newData); err != nil {
		return nil, err
	}
	return newData, nil
}

func (s *SLOServerComponent) CloneToClusters(ctx context.Context, req *slov1.MultiClusterSLO) (*slov1.MultiClusterFailures, error) {
	existing, err := s.storage.Get().SLOs.Get(ctx, path.Join("/slos", req.CloneId.Id))
	if err != nil {
		return nil, err
	}
	if err := existing.Validate(); err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasources[existing.SLO.GetDatasource()]
	for _, cluster := range req.Clusters {
		if err := sloStore.Precondition(ctx, cluster); err != nil {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
	}
	failures := []string{}
	createdIds, createdData, errors := sloStore.MultiClusterClone(ctx, existing, req.Clusters)
	createdAt := timestamppb.New(time.Now())
	for idx, err := range errors {
		if err != nil {
			failures = append(failures, err.Error())
		} else {
			createdData[idx].CreatedAt = createdAt
			if err := s.storage.Get().SLOs.Put(
				ctx,
				path.Join("/slos", createdIds[idx].Id),
				createdData[idx],
			); err != nil {
				failures = append(failures, fmt.Sprintf("failed to persist SLO configuration : %s", err.Error()))
			}
		}
	}
	return &slov1.MultiClusterFailures{
		Failures: failures,
	}, nil
}

func (s *SLOServerComponent) GetSLOStatus(ctx context.Context, ref *corev1.Reference) (*slov1.SLOStatus, error) {
	existing, err := s.storage.Get().SLOs.Get(ctx, path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasources[existing.SLO.GetDatasource()]
	if err := sloStore.Precondition(ctx, &corev1.Reference{Id: existing.SLO.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	status, err := sloStore.Status(ctx, existing)
	return status, err
}

func (s *SLOServerComponent) Preview(ctx context.Context, req *slov1.CreateSLORequest) (*slov1.SLOPreviewResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := checkDatasource(req.GetSlo().GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasources[req.GetSlo().GetDatasource()]
	if err := sloStore.Precondition(ctx, &corev1.Reference{Id: req.Slo.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	preview, err := sloStore.Preview(ctx, req)
	return preview, err
}

// -------- Service Discovery ---------

func (s *SLOServerComponent) ListServices(ctx context.Context, req *slov1.ListServicesRequest) (*slov1.ServiceList, error) {
	err := checkDatasource(req.Datasource)
	if err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	datasourceMu.RLock()
	backend := datasources[req.Datasource]
	datasourceMu.RUnlock()

	if err := backend.Precondition(ctx, &corev1.Reference{Id: req.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return backend.ListServices(ctx, req)
}

func (s *SLOServerComponent) ListMetrics(ctx context.Context, req *slov1.ListMetricsRequest) (*slov1.MetricGroupList, error) {
	err := checkDatasource(req.Datasource)
	if err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	datasourceMu.RLock()
	backend := datasources[req.Datasource]
	datasourceMu.RUnlock()

	if err := backend.Precondition(ctx, &corev1.Reference{Id: req.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return backend.ListMetrics(ctx, req)
}

func (s *SLOServerComponent) ListEvents(ctx context.Context, req *slov1.ListEventsRequest) (*slov1.EventList, error) {
	// fetch labels & their label values for the given cluster & service
	if err := req.Validate(); err != nil {
		return nil, err
	}
	datasource := req.GetDatasource()
	if err := checkDatasource(datasource); err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	datasourceMu.RLock()
	backend := datasources[req.Datasource]
	datasourceMu.RUnlock()

	if err := backend.Precondition(ctx, &corev1.Reference{Id: req.ClusterId}); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return backend.ListEvents(ctx, req)
}

func (s *SLOServerComponent) ListDatasources(_ context.Context, _ *emptypb.Empty) (*slov1.DatasourceInfo, error) {
	datasourceMu.RLock()
	datasources := lo.Keys(datasources)
	defer datasourceMu.RUnlock()
	return &slov1.DatasourceInfo{
		Items: datasources,
	}, nil
}
