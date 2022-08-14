package slo

import (
	"context"
	"fmt"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

// Apply Cortex Rules to Cortex separately :
// - recording rules
// - metadata rules
// - alert rules
func applyCortexSLORules(
	p *Plugin,
	lg hclog.Logger,
	ctx context.Context,
	clusterId string,
	ruleSpec RuleGroupYAMLv2,
) error {
	out, err := yaml.Marshal(ruleSpec)
	if err != nil {
		return err
	}

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		YamlContent: string(out),
		ClusterId:   clusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load rules for cluster %s, rule : %s,",
			clusterId, string(out)))
	}
	return err
}

// }
func deleteCortexSLORules(
	p *Plugin,
	lg hclog.Logger,
	ctx context.Context,
	clusterId string,
	groupName string,
) error {
	_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		ClusterId: clusterId,
		GroupName: groupName,
	})
	// we can ignore 404s here since if we can't find them,
	// then it will be impossible to delete them anyway
	if err != nil && status.Code(err) != codes.NotFound {
		lg.Error(fmt.Sprintf("Failed to delete rule group with clusterId, groupName (%v %v): %v", clusterId, groupName, err))
		return err
	}
	return nil
}

//
//// Convert OpenSLO specs to Cortex Rule Groups & apply them
//func applyMonitoringSLODownstream(osloSpec oslov1.SLO, service *sloapi.Service, existingId string,
//	p *Plugin, slorequest *sloapi.CreateSLORequest, ctx context.Context, lg hclog.Logger) ([]*sloapi.SLOData, error) {
//	slogroup, err := ParseToPrometheusModel(osloSpec)
//	if err != nil {
//		lg.Error("failed to parse prometheus model IR :", err)
//		return nil, err
//	}
//
//	returnedSloImpl := []*sloapi.SLOData{}
//	rw, err := GeneratePrometheusNoSlothGenerator(slogroup, slorequest.SLO.BudgetingInterval.AsDuration(), existingId, ctx, lg)
//	if err != nil {
//		lg.Error("Failed to generate prometheus : ", err)
//		return nil, err
//	}
//	lg.Debug(fmt.Sprintf("Generated cortex rule groups : %d", len(rw)))
//	if len(rw) > 1 {
//		lg.Warn("Multiple cortex rule groups being applied")
//	}
//	for _, rwgroup := range rw {
//
//		actualID := rwgroup.ActualId
//
//		cortexRules, err := toCortexRequest(rwgroup, actualID)
//		if err != nil {
//			return nil, err
//		}
//		err = applyCortexSLORules(p, cortexRules, service, actualID, ctx, lg)
//
//		if err == nil {
//			dataToPersist := &sloapi.SLOData{
//				Id:      actualID,
//				SLO:     slorequest.SLO,
//				Service: service,
//			}
//			returnedSloImpl = append(returnedSloImpl, dataToPersist)
//		} else { // clean up any create rule groups
//			err = deleteCortexSLORules(p, actualID, service.ClusterId, ctx, lg)
//		}
//	}
//	return returnedSloImpl, nil
//}
