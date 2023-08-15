package alarms

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"go.uber.org/multierr"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	metadataReadOnly = "readOnly"
)

func ignoreOpniConfigurations(conf *alertingv1.AlertCondition) {
	conf.Metadata = nil
	conf.Name = ""
	conf.Description = ""
	conf.Labels = nil
	conf.Severity = 0
	conf.AttachedEndpoints = nil
	conf.Silence = nil
	conf.LastUpdated = nil
	conf.GoldenSignal = 0
	conf.OverrideType = ""
}

func applyMutableReadOnlyFields(dest, src *alertingv1.AlertCondition) {
	dest.Name = src.Name
	dest.Description = src.Description
	dest.Labels = src.Labels
	dest.Severity = src.Severity
	dest.AttachedEndpoints = src.AttachedEndpoints
	dest.Silence = src.Silence
	dest.LastUpdated = src.LastUpdated
	dest.GoldenSignal = src.GoldenSignal
	dest.OverrideType = src.OverrideType
}

func areRuleSpecsEqual(old, new *alertingv1.AlertCondition) bool {
	oldIgnoreOpniConf := util.ProtoClone(old)
	newIgnoreOpniConf := util.ProtoClone(new)
	ignoreOpniConfigurations(oldIgnoreOpniConf)
	ignoreOpniConfigurations(newIgnoreOpniConf)
	return cmp.Equal(oldIgnoreOpniConf, newIgnoreOpniConf, protocmp.Transform())

}

func (a *AlarmServerComponent) SyncRules(ctx context.Context, rules *rules.RuleManifest) (*emptypb.Empty, error) {
	clusterId := cluster.StreamAuthorizedID(ctx)
	errors := []error{}
	condStorage := a.conditionStorage.Get()
	for _, rule := range rules.Rules {
		incomingCond := &alertingv1.AlertCondition{
			// immutable sync fields
			Id:      rule.RuleId.Id,
			GroupId: rule.GroupId.Id,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
					PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
						ClusterId: &corev1.Reference{Id: clusterId},
						Query:     rule.GetExpr(),
						For:       rule.GetDuration(),
					},
				},
			},
			Metadata: map[string]string{
				metadataReadOnly: "true",
			},
			// these will be considered mutable
			Name:              rule.Name,
			Description:       fmt.Sprintf("Prometheus alerting rule '%s' synced ", rule.Name),
			Labels:            []string{},
			Severity:          0,
			AttachedEndpoints: &alertingv1.AttachedEndpoints{},
			Silence:           &alertingv1.SilenceInfo{},
			LastUpdated:       timestamppb.Now(),
			GoldenSignal:      0,
			OverrideType:      "",
		}

		existing, err := condStorage.Group(rule.GetGroupId().Id).Get(ctx, rule.GetRuleId().Id)
		if err == nil {
			if !areRuleSpecsEqual(existing, incomingCond) {
				applyMutableReadOnlyFields(incomingCond, existing)
				if err := condStorage.Group(rule.GroupId.Id).Put(ctx, rule.RuleId.Id, incomingCond); err != nil {
					errors = append(errors, err)
				}
			}
		} else {
			if err := condStorage.Group(rule.GroupId.Id).Put(ctx, rule.RuleId.Id, incomingCond); err != nil {
				errors = append(errors, err)
			}
		}
	}
	return &emptypb.Empty{}, multierr.Combine(errors...)
}
