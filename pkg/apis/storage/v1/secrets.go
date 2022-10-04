package v1

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const Redacted = "***"

func (cfg *StorageSpec) RedactSecrets() {
	if cfg == nil {
		return
	}
	if cfg.S3 != nil {
		cfg.S3.RedactSecrets()
	}
	if cfg.Gcs != nil {
		cfg.Gcs.RedactSecrets()
	}
	if cfg.Azure != nil {
		cfg.Azure.RedactSecrets()
	}
	if cfg.Swift != nil {
		cfg.Swift.RedactSecrets()
	}
}

func (cfg *S3StorageSpec) RedactSecrets() {
	if cfg.SecretAccessKey != "" {
		cfg.SecretAccessKey = Redacted
	}
	if cfg.Sse != nil {
		cfg.Sse.RedactSecrets()
	}
}

func (cfg *GCSStorageSpec) RedactSecrets() {
	if cfg.ServiceAccount != "" {
		cfg.ServiceAccount = Redacted
	}
}

func (cfg *AzureStorageSpec) RedactSecrets() {
	if cfg.StorageAccountKey != "" {
		cfg.StorageAccountKey = Redacted
	}
	if cfg.MsiResource != "" {
		cfg.MsiResource = Redacted
	}
}

func (cfg *SwiftStorageSpec) RedactSecrets() {
	if cfg.Password != "" {
		cfg.Password = Redacted
	}
}

func (cfg *SSEConfig) RedactSecrets() {
	if cfg.KmsEncryptionContext != "" {
		cfg.KmsEncryptionContext = Redacted
	}
}

func (cfg *StorageSpec) UnredactSecrets(unredacted *StorageSpec) error {
	if cfg == nil {
		return nil
	}
	if cfg.S3 != nil {
		if err := cfg.S3.UnredactSecrets(unredacted.GetS3()); err != nil {
			return err
		}
	}
	if cfg.Gcs != nil {
		if err := cfg.Gcs.UnredactSecrets(unredacted.GetGcs()); err != nil {
			return err
		}
	}
	if cfg.Azure != nil {
		if err := cfg.Azure.UnredactSecrets(unredacted.GetAzure()); err != nil {
			return err
		}
	}
	if cfg.Swift != nil {
		if err := cfg.Swift.UnredactSecrets(unredacted.GetSwift()); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *S3StorageSpec) UnredactSecrets(unredacted *S3StorageSpec) error {
	if cfg.SecretAccessKey == Redacted {
		if unredacted.GetSecretAccessKey() == "" {
			return status.Error(codes.FailedPrecondition, "no secret access key provided")
		}
		cfg.SecretAccessKey = unredacted.SecretAccessKey
	}
	if cfg.Sse != nil {
		if err := cfg.Sse.UnredactSecrets(unredacted.GetSse()); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *GCSStorageSpec) UnredactSecrets(unredacted *GCSStorageSpec) error {
	if cfg.ServiceAccount == Redacted {
		if unredacted.GetServiceAccount() == "" {
			return status.Error(codes.FailedPrecondition, "no service account provided")
		}
		cfg.ServiceAccount = unredacted.ServiceAccount
	}
	return nil
}

func (cfg *AzureStorageSpec) UnredactSecrets(unredacted *AzureStorageSpec) error {
	if cfg.StorageAccountKey == Redacted {
		if unredacted.GetStorageAccountKey() == "" {
			return status.Error(codes.FailedPrecondition, "no storage account key provided")
		}
		cfg.StorageAccountKey = unredacted.StorageAccountKey
	}
	if cfg.MsiResource == Redacted {
		if unredacted.GetMsiResource() == "" {
			return status.Error(codes.FailedPrecondition, "no MSI resource provided")
		}
		cfg.MsiResource = unredacted.MsiResource
	}
	return nil
}

func (cfg *SwiftStorageSpec) UnredactSecrets(unredacted *SwiftStorageSpec) error {
	if cfg.Password == Redacted {
		if unredacted.GetPassword() == "" {
			return status.Error(codes.FailedPrecondition, "no password provided")
		}
		cfg.Password = unredacted.Password
	}
	return nil
}

func (cfg *SSEConfig) UnredactSecrets(unredacted *SSEConfig) error {
	if cfg.KmsEncryptionContext == Redacted {
		if unredacted.GetKmsEncryptionContext() == "" {
			return status.Error(codes.FailedPrecondition, "no KMS encryption context provided")
		}
		cfg.KmsEncryptionContext = unredacted.KmsEncryptionContext
	}
	return nil
}
