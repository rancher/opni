package loggingadmin

import "github.com/rancher/opni/pkg/validation"

func (o *OpensearchClusterV2) Validate() error {
	if o.S3 != nil {
		if err := o.S3.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (o *OpensearchS3Settings) Validate() error {
	if o.Endpoint == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Endpoint")
	}
	if o.Credentials != nil {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Credentials")
	}
	if o.Credentials.AccessKey != "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "AccessKey")
	}
	if o.Credentials.SecretKey != "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "SecretKey")
	}
	if o.Bucket == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Bucket")
	}
	if o.Folder != nil {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Folder")
	}
	if *o.Folder == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Folder")
	}
	return nil
}
