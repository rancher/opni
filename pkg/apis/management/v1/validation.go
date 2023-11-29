package v1

import (
	"encoding/json"
	"fmt"

	"github.com/rancher/opni/pkg/validation"
)

func (r *CreateBootstrapTokenRequest) Validate() error {
	if err := r.GetTtl().CheckValid(); err != nil {
		return fmt.Errorf("%w (ttl): %s", validation.ErrInvalidValue, err.Error())
	}
	if r.GetTtl().AsDuration() <= 0 {
		return fmt.Errorf("%w: %s", validation.ErrInvalidValue, "ttl cannot be negative or zero")
	}
	if err := validation.ValidateLabels(r.GetLabels()); err != nil {
		return err
	}
	for _, cap := range r.GetCapabilities() {
		if err := validation.Validate(cap); err != nil {
			return err
		}
	}
	return nil
}

func (r *ListClustersRequest) Validate() error {
	if r.MatchLabels != nil {
		if err := validation.Validate(r.MatchLabels); err != nil {
			return err
		}
	}
	if err := validation.Validate(r.MatchOptions); err != nil {
		return err
	}
	return nil
}

func (r *EditClusterRequest) Validate() error {
	if r.Cluster == nil {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "cluster")
	}
	if err := validation.Validate(r.Cluster); err != nil {
		return err
	}
	if err := validation.ValidateLabels(r.GetLabels()); err != nil {
		return err
	}
	return nil
}

func (r *WatchClustersRequest) Validate() error {
	for _, c := range r.GetKnownClusters().GetItems() {
		if err := validation.Validate(c); err != nil {
			return err
		}
	}
	return nil
}

func (r *UpdateConfigRequest) Validate() error {
	if len(r.Documents) == 0 {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "documents")
	}
	for i, doc := range r.Documents {
		if ok := json.Valid(doc.Json); !ok {
			return fmt.Errorf("%w: malformed json in document %d", validation.ErrInvalidValue, i)
		}
	}
	return nil
}

func (r *CapabilityInstallRequest) Validate() error {
	if err := validation.ValidateID(r.Name); err != nil {
		return err
	}
	if err := validation.Validate(r.Target); err != nil {
		return err
	}
	return nil
}

func (r *CapabilityUninstallRequest) Validate() error {
	if err := validation.ValidateID(r.Name); err != nil {
		return err
	}
	if err := validation.Validate(r.Target); err != nil {
		return err
	}
	return nil
}

func (r *CapabilityStatusRequest) Validate() error {
	if err := validation.ValidateID(r.Name); err != nil {
		return err
	}
	if err := validation.Validate(r.Cluster); err != nil {
		return err
	}
	return nil
}

func (r *CapabilityUninstallCancelRequest) Validate() error {
	if err := validation.ValidateID(r.Name); err != nil {
		return err
	}
	if err := validation.Validate(r.Cluster); err != nil {
		return err
	}
	return nil
}

func (r *StreamAgentLogsRequest) Validate() error {
	if err := validation.Validate(r.Agent); err != nil {
		return err
	}

	if err := validation.Validate(r.Request); err != nil {
		return err
	}

	return nil
}
