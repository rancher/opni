package slo

import (
	"context"
	"fmt"

	oslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/google/uuid"
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
	if err != nil {
		return nil, err
	}

	rmetadata, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s%s", sloId, MetadataRuleSuffix),
		Rules: metadata,
	})
	if err != nil {
		return nil, err
	}

	ralerts, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s%s", sloId, AlertRuleSuffix),
		Rules: alerts,
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

// Cortex applies rule groups individually
func applyCortexSLORules(p *Plugin, cortexRules *CortexRuleWrapper, service *sloapi.Service, existingId string, ctx context.Context, lg hclog.Logger) error {
	var anyError error
	_, err := p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
		Yaml:   cortexRules.recording,
		Tenant: service.ClusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load recording rules for cluster %s, service %s, id %s : %v",
			service.ClusterId, service.JobId, existingId, anyError))
		anyError = err
	}
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
		Yaml:   cortexRules.metadata,
		Tenant: service.ClusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load metadata rules for cluster %s, service %s, id %s : %v",
			service.ClusterId, service.JobId, existingId, anyError))
		anyError = err
	}
	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
		Yaml:   cortexRules.alerts,
		Tenant: service.ClusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load alerting rules for cluster %s, service %s, id %s : %v",
			service.ClusterId, service.JobId, existingId, anyError))
		anyError = err
	}
	return anyError
}

func deleteCortexSLORules(p *Plugin, toDelete *sloapi.SLOData, ctx context.Context, lg hclog.Logger) error {
	id, clusterId := toDelete.Id, toDelete.Service.ClusterId
	var anyError error
	_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		Tenant:    clusterId,
		GroupName: id + RecordingRuleSuffix,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to delete recording rule group with id  %v: %v", id, err))
		anyError = err
	}
	_, err = p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		Tenant:    clusterId,
		GroupName: id + MetadataRuleSuffix,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to delete metadata rule group with id  %v: %v", id, err))
		anyError = err
	}
	_, err = p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		Tenant:    clusterId,
		GroupName: id + AlertRuleSuffix,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to delete alerting rule group with id  %v: %v", id, err))
		anyError = err
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
	rw, err := GeneratePrometheusNoSlothGenerator(slogroup, slorequest.SLO.BudgetingInterval.AsDuration(), ctx, lg)
	if err != nil {
		lg.Error("Failed to generate prometheus : ", err)
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Generated cortex rule groups : %d", len(rw)))
	if len(rw) > 1 {
		lg.Warn("Multiple cortex rule groups being applied")
	}
	for _, rwgroup := range rw {
		// Generate new uuid for new slo
		if existingId == "" {
			existingId = uuid.New().String()
		}

		cortexRules, err := toCortexRequest(rwgroup, existingId)
		if err != nil {
			return nil, err
		}
		applyCortexSLORules(p, cortexRules, service, existingId, ctx, lg)

		dataToPersist := &sloapi.SLOData{
			Id:      existingId,
			SLO:     slorequest.SLO,
			Service: service,
		}
		returnedSloImpl = append(returnedSloImpl, dataToPersist)
	}
	return returnedSloImpl, nil
}
