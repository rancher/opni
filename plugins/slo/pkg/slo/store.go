package slo

import (
	"context"
	"fmt"
	"os"

	oslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"gopkg.in/yaml.v3"
)

var (
	RecordingRuleSuffix = "-recording"
	MetadataRuleSuffix  = "-metadata"
	AlertRuleSuffix     = "-alerts"
)

// these types are defined to support yaml v2 (instead of the new Prometheus
// YAML v3 that has some problems with marshaling).
type ruleGroupsYAMLv2 struct {
	Groups []ruleGroupYAMLv2 `yaml:"groups"`
}

type ruleGroupYAMLv2 struct {
	Name     string             `yaml:"name"`
	Interval prommodel.Duration `yaml:"interval,omitempty"`
	Rules    []rulefmt.Rule     `yaml:"rules"`
}

// Guess this could be generic
type zipperHolder struct {
	Spec    *oslov1.SLO
	Service *apis.Service
}

type CortexRuleWrapper struct {
	recording string
	metadata  string
	alerts    string
}

func zipOpenSLOWithServices(ps []oslov1.SLO, as []*apis.Service) ([]*zipperHolder, error) {
	if len(as) != len(ps) {
		return nil, fmt.Errorf("Expected Generated SLOGroups to match the number of Services provided in the request")
	}
	res := make([]*zipperHolder, 0)
	for idx, p := range ps {
		res = append(res, &zipperHolder{
			Spec:    &p,
			Service: as[idx],
		})
	}
	return res, nil
}

// Marshal result SLORuleFmtWrapper to cortex-approved yaml
func toCortexRequest(rw SLORuleFmtWrapper, sloId string) (*CortexRuleWrapper, error) {

	recording, metadata, alerts := rw.SLIrules, rw.MetaRules, rw.AlertRules
	// Check length is 0?
	rrecording, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s%s", sloId, RecordingRuleSuffix),
		Rules: recording,
	})
	os.WriteFile(fmt.Sprintf("%s%s.yaml", sloId, RecordingRuleSuffix), rrecording, 0644)
	if err != nil {
		return nil, err
	}

	rmetadata, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s%s", sloId, MetadataRuleSuffix),
		Rules: metadata,
	})
	os.WriteFile(fmt.Sprintf("%s%s.yaml", sloId, MetadataRuleSuffix), rmetadata, 0644)
	if err != nil {
		return nil, err
	}

	ralerts, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s%s", sloId, AlertRuleSuffix),
		Rules: alerts,
	})
	os.WriteFile(fmt.Sprintf("%s%s.yaml", sloId, AlertRuleSuffix), rmetadata, 0644)
	if err != nil {
		return nil, err
	}
	return &CortexRuleWrapper{
		recording: string(rrecording),
		metadata:  string(rmetadata),
		alerts:    string(ralerts),
	}, nil
}

// Cortex applies rule groups individually
func applyCortexSLORules(p *Plugin, cortexRules *CortexRuleWrapper, service *sloapi.Service, existingId string, ctx context.Context, lg hclog.Logger) error {
	var anyError error
	ruleGroupsToApply := []string{cortexRules.recording, cortexRules.metadata, cortexRules.alerts}
	for _, ruleGroup := range ruleGroupsToApply {
		_, err := p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
			Yaml:   ruleGroup,
			Tenant: service.ClusterId,
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

func deleteCortexSLORules(p *Plugin, toDelete *sloapi.SLOData, ctx context.Context, lg hclog.Logger) error {
	id, clusterId := toDelete.Id, toDelete.Service.ClusterId
	ruleGroupsToDelete := []string{id + RecordingRuleSuffix, id + MetadataRuleSuffix, id + AlertRuleSuffix}
	var anyError error

	for _, ruleGroup := range ruleGroupsToDelete {
		_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
			Tenant:    clusterId,
			GroupName: ruleGroup,
		})
		if err != nil {
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
		applyCortexSLORules(p, cortexRules, service, actualID, ctx, lg)

		dataToPersist := &sloapi.SLOData{
			Id:      actualID,
			SLO:     slorequest.SLO,
			Service: service,
		}
		returnedSloImpl = append(returnedSloImpl, dataToPersist)
	}
	return returnedSloImpl, nil
}
