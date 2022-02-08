package core

import (
	"fmt"

	"github.com/rancher/opni-monitoring/pkg/validation"
)

func (c *Cluster) Validate() error {
	if c.Id == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "id")
	}
	if err := validation.ValidateID(c.Id); err != nil {
		return err
	}
	if err := validation.ValidateLabels(c.Labels); err != nil {
		return err
	}
	return nil
}

func (ls *LabelSelector) Validate() error {
	if ls.MatchLabels != nil {
		if err := validation.ValidateLabels(ls.MatchLabels); err != nil {
			return err
		}
	}
	for _, l := range ls.MatchExpressions {
		if err := l.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r *LabelSelectorRequirement) Validate() error {
	if r.Key == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "key")
	}
	if r.Operator == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "operator")
	}
	if err := validation.ValidateLabelName(r.Key); err != nil {
		return err
	}
	switch LabelSelectorOperator(r.Operator) {
	case LabelSelectorOpIn, LabelSelectorOpNotIn, LabelSelectorOpExists, LabelSelectorOpDoesNotExist:
	default:
		return fmt.Errorf("%w: unknown operator %q (values are case-sensitive)", validation.ErrInvalidValue, r.Operator)
	}
	for _, value := range r.Values {
		if err := validation.ValidateLabelValue(value); err != nil {
			return err
		}
	}
	return nil
}

func (r *Role) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "name")
	}
	if err := validation.ValidateName(r.Name); err != nil {
		return fmt.Errorf("%w: %q", err, r.Name)
	}
	for _, clusterID := range r.ClusterIDs {
		if err := validation.ValidateID(clusterID); err != nil {
			return fmt.Errorf("%w: %q", err, clusterID)
		}
	}
	if r.MatchLabels != nil {
		if err := r.MatchLabels.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (rb *RoleBinding) Validate() error {
	if rb.Name == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "name")
	}
	if rb.RoleName == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "roleName")
	}
	if err := validation.ValidateName(rb.Name); err != nil {
		return fmt.Errorf("%w: %q", err, rb.Name)
	}
	if err := validation.ValidateName(rb.RoleName); err != nil {
		return fmt.Errorf("%w: %q", err, rb.RoleName)
	}
	for _, subject := range rb.Subjects {
		if err := validation.ValidateSubject(subject); err != nil {
			return err
		}
	}
	return nil
}

func (ref *Reference) Validate() error {
	if ref.Id == "" && ref.Name == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "at least one of ID or Name must be set")
	}
	if ref.Id != "" {
		if err := validation.ValidateID(ref.Id); err != nil {
			return err
		}
	}
	if ref.Name == "" {
		if err := validation.ValidateName(ref.Name); err != nil {
			return err
		}
	}
	return nil
}

func (sar *SubjectAccessRequest) Validate() error {
	if sar.Subject == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "subject")
	}
	if err := validation.ValidateSubject(sar.Subject); err != nil {
		return err
	}
	return nil
}
