package slo

import (
	"context"
	"fmt"
	oslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
	"time"
)

var (
	RecordingRuleSuffix = "-recording"
	MetadataRuleSuffix  = "-metadata"
	AlertRuleSuffix     = "-alerts"
)

type ruleGroupYAMLv2 struct {
	Name     string             `yaml:"name"`
	Interval prommodel.Duration `yaml:"interval,omitempty"`
	Rules    []rulefmt.Rule     `yaml:"rules"`
}

type CortexRuleWrapper struct {
	recording string
	metadata  string
	alerts    string
}

// Marshal result SLORuleFmtWrapper to cortex-approved yaml
func toCortexRequest(rw SLORuleFmtWrapper, sloId string) (*CortexRuleWrapper, error) {

	recording, metadata, alerts := rw.SLIrules, rw.MetaRules, rw.AlertRules
	// Check length is 0?
	interval, err := prommodel.ParseDuration(timeDurationToPromStr(time.Second))
	if err != nil {
		return nil, err
	}

	rrecording, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:     fmt.Sprintf("%s%s", sloId, RecordingRuleSuffix),
		Interval: interval,
		Rules:    recording,
	})
	//@here debug recording rules
	//os.WriteFile(fmt.Sprintf("recording-%s.yaml", sloId), rrecording, 0644)
	if err != nil {
		return nil, err
	}

	rmetadata, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:     fmt.Sprintf("%s%s", sloId, MetadataRuleSuffix),
		Interval: interval,
		Rules:    metadata,
	})
	if err != nil {
		return nil, err
	}

	ralerts, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:     fmt.Sprintf("%s%s", sloId, AlertRuleSuffix),
		Interval: interval,
		Rules:    alerts,
	})
	if err != nil {
		return nil, err
	}
	return &CortexRuleWrapper{
		recording: string(rrecording),
		metadata:  string(rmetadata),
		alerts:    string(ralerts),
	}, nil
}

// Apply Cortex Rules to Cortex separately :
// - recording rules
// - metadata rules
// - alert rules
func applyCortexSLORules(p *Plugin, cortexRules *CortexRuleWrapper, service *sloapi.Service, existingId string, ctx context.Context, lg hclog.Logger) error {
	var anyError error
	ruleGroupsToApply := []string{cortexRules.recording, cortexRules.metadata, cortexRules.alerts}
	for _, ruleGroup := range ruleGroupsToApply {
		_, err := p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
			YamlContent: ruleGroup,
			ClusterId:   service.ClusterId,
		})
		if err != nil {
			lg.Error(fmt.Sprintf(
				"Failed to load rules for cluster %s, service %s, id %s, rule %s : %v",
				service.ClusterId, service.JobId, existingId, ruleGroup, anyError))
			anyError = err
		}
	}
	return anyError
}

func deleteCortexSLORules(p *Plugin, id string, clusterId string, ctx context.Context, lg hclog.Logger) error {
	ruleGroupsToDelete := []string{id + RecordingRuleSuffix, id + MetadataRuleSuffix, id + AlertRuleSuffix}
	var anyError error

	for _, ruleGroup := range ruleGroupsToDelete {
		_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: clusterId,
			GroupName: ruleGroup,
		})
		// we can ignore 404s here since if we can't find them,
		// then it will be impossible to delete them anyway
		if err != nil && status.Code(err) != codes.NotFound {
			lg.Error(fmt.Sprintf("Failed to delete rule group with id  %v: %v", id, err))
			anyError = err
		}
	}
	return anyError
}

// Convert OpenSLO specs to Cortex Rule Groups & apply them
func applyMonitoringSLODownstream(osloSpec oslov1.SLO, service *sloapi.Service, existingId string,
	p *Plugin, slorequest *sloapi.CreateSLORequest, ctx context.Context, lg hclog.Logger) ([]*sloapi.SLOData, error) {
	slogroup, err := ParseToPrometheusModel(osloSpec)
	if err != nil {
		lg.Error("failed to parse prometheus model IR :", err)
		return nil, err
	}

	returnedSloImpl := []*sloapi.SLOData{}
	rw, err := GeneratePrometheusNoSlothGenerator(slogroup, slorequest.SLO.BudgetingInterval.AsDuration(), existingId, ctx, lg)
	if err != nil {
		lg.Error("Failed to generate prometheus : ", err)
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Generated cortex rule groups : %d", len(rw)))
	if len(rw) > 1 {
		lg.Warn("Multiple cortex rule groups being applied")
	}
	for _, rwgroup := range rw {

		actualID := rwgroup.ActualId

		cortexRules, err := toCortexRequest(rwgroup, actualID)
		if err != nil {
			return nil, err
		}
		err = applyCortexSLORules(p, cortexRules, service, actualID, ctx, lg)

		if err == nil {
			dataToPersist := &sloapi.SLOData{
				Id:      actualID,
				SLO:     slorequest.SLO,
				Service: service,
			}
			returnedSloImpl = append(returnedSloImpl, dataToPersist)
		} else { // clean up any create rule groups
			err = deleteCortexSLORules(p, actualID, service.ClusterId, ctx, lg)
		}
	}
	return returnedSloImpl, nil
}
