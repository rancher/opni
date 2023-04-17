package node

import (
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/validation"
)

func (s *SyncRequest) Validate() error {
	if s.CurrentConfig == nil {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "CurrentConfig")
	}
	if s.CurrentConfig.Spec == nil {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "CurrentConfig.Spec")
	}
	return s.CurrentConfig.Spec.Validate()
}

func (m *MetricsCapabilitySpec) Validate() error {
	if m.GetPrometheus() != nil && m.GetOtel() != nil {
		return validation.Errorf("Only one configuration can be set at a time: Prometheus, Otel")
	}
	if m.GetPrometheus() == nil && m.GetOtel() == nil {
		return validation.Errorf("one of Prometheus or Otel specs must be set")
	}

	if m.GetOtel() != nil {
		return m.GetOtel().Validate()
	}
	return nil
}

func (o *OTELSpec) Validate() error {
	if len(o.AdditionalScrapeConfigs) > 0 {
		for _, config := range o.AdditionalScrapeConfigs {
			if err := config.Validate(); err != nil {
				return err
			}
		}
	}
	if o.Wal != nil {
		if err := o.Wal.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (w *WALConfig) Validate() error {
	if w.Enabled {
		if w.BufferSize <= 0 {
			return validation.Error("WAL BufferSize must be greater than 0")
		}
		if w.TruncateFrequency == nil {
			return validation.Error("WALConfig TruncateFrequency must be set")
		}
	}
	return nil
}

func (s *ScrapeConfig) Validate() error {
	if s.JobName == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "JobName")
	}
	if len(s.Targets) == 0 {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Targets")
	}
	for _, t := range s.Targets {
		if t == "" {
			return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Target")
		}
	}
	if s.ScrapeInterval == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "ScrapeInterval")
	}
	_, err := model.ParseDuration(s.ScrapeInterval)
	if err != nil {
		return validation.Errorf("invalid ScrapeInterval: %s", err)
	}
	return nil
}
