package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"net/http"
	"path"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const conditionPrefix = "/alerting/conditions"

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	newId := uuid.New().String()
	_, err := setupCondition(ctx, req, newId)
	if err != nil {
		return nil, err
	}
	if err := setEndpointImplementationIfAvailable(p, ctx, req, newId); err != nil {
		return nil, err
	}
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, newId), req); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: newId}, nil
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, ref.Id))
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	keys, items, err := listWithKeys(ctx, p.storage.Get().Conditions, conditionPrefix)
	if err != nil {
		return nil, err
	}
	if len(keys) != len(items) {
		return nil, fmt.Errorf("Internal error : mismatched number of keys")
	}
	res := &alertingv1alpha.AlertConditionList{}
	for i := range keys {
		res.Items = append(res.Items, &alertingv1alpha.AlertConditionWithId{
			Id:             &corev1.Reference{Id: keys[i]},
			AlertCondition: items[i],
		})
	}
	return res, nil
}

// req.Id is the condition id reference
func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	overrideLabels := req.UpdateAlert.Labels

	if err := handleUpdateEndpointImplementation(p, ctx, req.Id.Id, existing, req.UpdateAlert); err != nil {
		return nil, err
	}
	// UPDATE THE ACTUAL CONDITION
	// until we have a more complicated setup, deleting then recreating is fine
	p.DeleteAlertCondition(ctx, &corev1.Reference{Id: req.Id.Id})
	_, err = setupCondition(ctx, req.UpdateAlert, req.Id.Id)
	if err != nil {
		return nil, err
	}

	proto.Merge(existing, req.UpdateAlert)
	existing.Labels = overrideLabels
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, req.Id.Id), existing); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, ref.Id))
	if err != nil {
		return nil, err
	}
	if err := deleteCondition(ctx, existing, ref.Id); err != nil {
		return nil, err
	}
	_, err = p.DeleteEndpointImplementation(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) PreviewAlertCondition(ctx context.Context,
	req *alertingv1alpha.PreviewAlertConditionRequest) (*alertingv1alpha.PreviewAlertConditionResponse, error) {
	// Create alert condition

	// measure status

	// Delete alert condition

	// return status
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ActivateSilence(ctx context.Context, req *alertingv1alpha.SilenceRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, req.ConditionId.Id))
	if err != nil {
		return nil, err
	}
	silence := &PostableSilence{}
	silence.WithCondition(req.ConditionId.Id)
	silence.WithDuration(req.Duration.AsDuration())
	if existing.Silence != nil { // the case where we are updating an existing silence
		silence.WithSilenceId(existing.Silence.SilenceId)
	}
	resp, err := PostSilence(ctx, p.alertingOptions.Get().Endpoints[0], silence)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound { // update failed
			// TODO specific shared.Err for status not found
		}
		return nil, fmt.Errorf("Failed to activate silence: %s", resp.Status)
	}
	respSilence := &PostSilencesResponse{}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			p.logger.Error(fmt.Sprintf("Failed to close response body %s", err))
		}
	}(resp.Body)
	if err := json.NewDecoder(resp.Body).Decode(respSilence); err != nil {
		return nil, err
	}
	// update existing proto with the silence info
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = &alertingv1alpha.SilenceInfo{
		SilenceId: respSilence.GetSilenceId(),
		StartsAt: &timestamppb.Timestamp{
			Seconds: silence.StartsAt.Unix(),
		},
		EndsAt: &timestamppb.Timestamp{
			Seconds: silence.EndsAt.Unix(),
		},
	}
	// update K,V with new silence info for the respective condition
	proto.Merge(existing, newCondition)
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, req.ConditionId.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DeactivateSilence req.Id is a condition id reference
func (p *Plugin) DeactivateSilence(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, req.Id))
	if err != nil {
		return nil, err
	}
	if existing.Silence == nil {
		return nil, validation.Errorf("Could not find existing silence for condition %s", req.Id)
	}
	silence := &DeletableSilence{
		silenceId: existing.Silence.SilenceId,
	}
	resp, err := DeleteSilence(ctx, p.alertingOptions.Get().Endpoints[0], silence)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to deactivate silence: %s", resp.Status)
	}
	// update existing proto with the silence info
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = nil
	// update K,V with new silence info for the respective condition
	proto.Merge(existing, newCondition)
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, req.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
