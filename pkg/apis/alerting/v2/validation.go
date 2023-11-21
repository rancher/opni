package v2

import "github.com/rancher/opni/pkg/validation"

func (r *OpniReceiver) Validate() error {
	if r == nil {
		return validation.Error("Input is nil")
	}
	if r.Receiver == nil {
		return validation.Error("field receiver is required")
	}
	if err := r.Receiver.Validate(); err != nil {
		return err
	}
	for _, lm := range r.LabelMatchers {
		if err := lm.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a *AlertPayload) Validate() error {
	if len(a.GetLabels().GetLabels()) == 0 {
		return validation.Error("one ore more labels are required")
	}

	if len(a.GetClusters()) == 0 {
		return validation.Error("one ore more clusters are required")
	}
	return nil
}

func (l *LabelMatcher) Validate() error {
	if l == nil {
		return validation.Error("LabelMatcher is nil")
	}
	if l.Name == "" {
		return validation.Error("field name is required")
	}
	return nil
}
