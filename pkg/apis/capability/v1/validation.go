package v1

import "github.com/rancher/opni/pkg/validation"

func (r *InstallRequest) Validate() error {
	if err := validation.Validate(r.Capability); err != nil {
		return err
	}
	if err := validation.Validate(r.Agent); err != nil {
		return err
	}
	return nil
}

func (r *UninstallRequest) Validate() error {
	if err := validation.Validate(r.Capability); err != nil {
		return err
	}
	if err := validation.Validate(r.Agent); err != nil {
		return err
	}
	return nil
}

func (r *StatusRequest) Validate() error {
	if err := validation.Validate(r.Capability); err != nil {
		return err
	}
	if err := validation.Validate(r.Agent); err != nil {
		return err
	}
	return nil
}

func (r *UninstallStatusRequest) Validate() error {
	if err := validation.Validate(r.Capability); err != nil {
		return err
	}
	if err := validation.Validate(r.Agent); err != nil {
		return err
	}
	return nil
}

func (r *CancelUninstallRequest) Validate() error {
	if err := validation.Validate(r.Capability); err != nil {
		return err
	}
	if err := validation.Validate(r.Agent); err != nil {
		return err
	}
	return nil
}
