package slo

import (
	"fmt"
	"strings"

	"github.com/alexandreLamarre/sloth/core/prometheus"
	"github.com/kralicky/yaml/v3"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	apis "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
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
	SLOGroup *prometheus.SLOGroup
	Service  *apis.Service
}

func zipPrometheusModelWithServices(ps []*prometheus.SLOGroup, as []*apis.Service) ([]*zipperHolder, error) {
	if len(as) != len(ps) {
		return nil, fmt.Errorf("Expected Generated SLOGroups to match the number of Services provided in the request")
	}
	res := make([]*zipperHolder, 0)
	for idx, p := range ps {
		res = append(res, &zipperHolder{
			SLOGroup: p,
			Service:  as[idx],
		})
	}
	return res, nil
}

// Marshal result SLORuleFmtWrapper to cortex-approved yaml
func toCortexRequest(rw SLORuleFmtWrapper, sloId string) (string, error) {

	recording, metadata, alerts := rw.SLIrules, rw.MetaRules, rw.AlertRules
	// Check length is 0?
	rrecording, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s-recording", sloId),
		Rules: recording,
	})
	if err != nil {
		return "", err
	}

	rmetadata, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s-metadata", sloId),
		Rules: metadata,
	})
	if err != nil {
		return "", err
	}

	ralerts, err := yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s-alerts", sloId),
		Rules: alerts,
	})
	if err != nil {
		return "", err
	}
	res := strings.Join([]string{string(rrecording), string(rmetadata), string(ralerts)}, "---\n")
	return res, nil
}
