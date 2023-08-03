package system

import "github.com/rancher/opni/pkg/validation"

func (pr *PutRequest) Validate() error {
	if pr.Key == "" {
		return validation.Errorf("%w: key", validation.ErrMissingRequiredField)
	}
	if pr.Value == nil {
		return validation.Errorf("%w: value", validation.ErrMissingRequiredField)
	}
	return nil
}

func (gr *GetRequest) Validate() error {
	if gr.Key == "" {
		return validation.Errorf("%w: key", validation.ErrMissingRequiredField)
	}
	return nil
}

func (dr *DeleteRequest) Validate() error {
	if dr.Key == "" {
		return validation.Errorf("%w: key", validation.ErrMissingRequiredField)
	}
	return nil
}

func (lr *ListKeysRequest) Validate() error {
	if lr.Key == "" {
		return validation.Errorf("%w: key", validation.ErrMissingRequiredField)
	}
	if lr.GetLimit() < 0 {
		return validation.Errorf("%w: limit must be >= 0", validation.ErrInvalidValue)
	}
	return nil
}

func (hr *HistoryRequest) Validate() error {
	if hr.Key == "" {
		return validation.Errorf("%w: key", validation.ErrMissingRequiredField)
	}
	return nil
}
