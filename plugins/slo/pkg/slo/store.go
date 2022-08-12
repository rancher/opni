package slo

import (
	"fmt"
	prommodel "github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

type CortexRuleWrapper struct {
	recording string
	metadata  string
	alerts    string
}

// Marshal result SLORuleFmtWrapper to cortex-approved yaml
func toCortexRequest(rw SLORuleFmtWrapper, sloId string) (*CortexRuleWrapper, error) {

	recording, metadata, alerts := rw.SLIrules, rw.MetaRules, rw.AlertRules
	// Check length is 0?
	interval, err := prommodel.ParseDuration(TimeDurationToPromStr(time.Second))
	if err != nil {
		return nil, err
	}

	rrecording, err := yaml.Marshal(RuleGroupYAMLv2{
		Name:     fmt.Sprintf("%s%s", sloId, RecordingRuleSuffix),
		Interval: interval,
		Rules:    recording,
	})
	//@here debug recording rules
	os.WriteFile(fmt.Sprintf("recording-%s.yaml", sloId), rrecording, 0644)
	if err != nil {
		return nil, err
	}

	rmetadata, err := yaml.Marshal(RuleGroupYAMLv2{
		Name:     fmt.Sprintf("%s%s", sloId, MetadataRuleSuffix),
		Interval: interval,
		Rules:    metadata,
	})
	if err != nil {
		return nil, err
	}
	os.WriteFile(fmt.Sprintf("metadata-%s.yaml", sloId), rmetadata, 0644)

	ralerts, err := yaml.Marshal(RuleGroupYAMLv2{
		Name:     fmt.Sprintf("%s%s", sloId, AlertRuleSuffix),
		Interval: interval,
		Rules:    alerts,
	})
	if err != nil {
		return nil, err
	}
	os.WriteFile(fmt.Sprintf("alerts-%s.yaml", sloId), ralerts, 0644)
	return &CortexRuleWrapper{
		recording: string(rrecording),
		metadata:  string(rmetadata),
		alerts:    string(ralerts),
	}, nil
}
