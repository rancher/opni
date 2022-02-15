package management

import (
	"encoding/json"

	"github.com/rancher/opni-monitoring/pkg/validation"
)

func (r *CreateBootstrapTokenRequest) Validate() error {
	if err := r.GetTtl().CheckValid(); err != nil {
		return validation.Error(err.Error())
	}
	if r.GetTtl().AsDuration() <= 0 {
		return validation.Error("ttl cannot be negative or zero")
	}
	if err := validation.ValidateLabels(r.GetLabels()); err != nil {
		return err
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
		return validation.Error("no config documents provided")
	}
	for i, doc := range r.Documents {
		if ok := json.Valid(doc.Json); !ok {
			return validation.Errorf("malformed json in document %d", i)
		}
	}
	return nil
}
