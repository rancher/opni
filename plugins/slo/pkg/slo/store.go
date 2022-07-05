package slo

import (
	"fmt"
	"strings"

	"github.com/kralicky/yaml/v3"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
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

	_, err = yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s-metadata", sloId),
		Rules: metadata,
	})
	if err != nil {
		return "", err
	}

	_, err = yaml.Marshal(ruleGroupYAMLv2{
		Name:  fmt.Sprintf("%s-alerts", sloId),
		Rules: alerts,
	})
	if err != nil {
		return "", err
	}
	res := strings.Join([]string{string(rrecording) /*string(rmetadata), string(ralerts)*/}, "---\n")
	return res, nil
}
