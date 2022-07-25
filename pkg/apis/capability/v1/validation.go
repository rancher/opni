package v1

import "github.com/rancher/opni/pkg/validation"

func (req *UninstallRequest) Validate() error {
	if err := validation.Validate(req.Cluster); err != nil {
		return err
	}
	return nil
}

func (req *InstallRequest) Validate() error {
	if err := validation.Validate(req.Cluster); err != nil {
		return err
	}
	return nil
}
