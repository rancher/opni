package v1

import (
	"fmt"

	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
)

func (c *Cluster) Validate() error {
	if c.Id == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "id")
	}
	if err := validation.ValidateID(c.Id); err != nil {
		return err
	}
	if err := validation.ValidateLabels(c.GetMetadata().GetLabels()); err != nil {
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

func (s *ClusterSelector) Validate() error {
	for _, clusterID := range s.ClusterIDs {
		if err := validation.ValidateID(clusterID); err != nil {
			return err
		}
	}
	if len(lo.Uniq(s.ClusterIDs)) != len(s.ClusterIDs) {
		return fmt.Errorf("%w: %s", validation.ErrDuplicate, "clusterIDs")
	}
	if s.LabelSelector != nil {
		if err := s.LabelSelector.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Role) Validate() error {
	if r.Id == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "id")
	}
	if err := validation.ValidateID(r.Id); err != nil {
		return fmt.Errorf("%w: %q", err, r.Id)
	}
	for _, clusterID := range r.ClusterIDs {
		if err := validation.ValidateID(clusterID); err != nil {
			return fmt.Errorf("%w: %q", err, clusterID)
		}
	}
	if len(lo.Uniq(r.ClusterIDs)) != len(r.ClusterIDs) {
		return fmt.Errorf("%w: %s", validation.ErrDuplicate, "clusterIDs")
	}
	if r.MatchLabels != nil {
		if err := r.MatchLabels.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (rb *RoleBinding) Validate() error {
	if rb.Id == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "id")
	}
	if rb.RoleId == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "roleId")
	}
	if err := validation.ValidateID(rb.Id); err != nil {
		return fmt.Errorf("%w: %q", err, rb.Id)
	}
	if err := validation.ValidateID(rb.RoleId); err != nil {
		return fmt.Errorf("%w: %q", err, rb.RoleId)
	}
	for _, subject := range rb.Subjects {
		if err := validation.ValidateSubject(subject); err != nil {
			return err
		}
	}
	return nil
}

func (ref *Reference) Validate() error {
	if ref.Id == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "id")
	}
	if err := validation.ValidateID(ref.Id); err != nil {
		return err
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

func (o MatchOptions) Validate() error {
	if _, ok := MatchOptions_name[int32(o)]; !ok {
		return fmt.Errorf("%w: MatchOptions(%d)", validation.ErrInvalidValue, o)
	}
	return nil
}

func (tc *TokenCapability) Validate() error {
	if tc.Type == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "type")
	}
	if ref := tc.GetReference(); ref != nil {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (cc *ClusterCapability) Validate() error {
	if cc.Name == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "name")
	}
	return nil
}
