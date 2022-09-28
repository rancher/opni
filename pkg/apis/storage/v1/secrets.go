package v1

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

func (cfg *StorageSpec) UnredactSecrets(unredacted *StorageSpec) {
	if cfg == nil || unredacted == nil {
		return
	}
	if cfg.S3 != nil && unredacted.S3 != nil {
		cfg.S3.UnredactSecrets(unredacted.S3)
	}
	if cfg.Gcs != nil && unredacted.Gcs != nil {
		cfg.Gcs.UnredactSecrets(unredacted.Gcs)
	}
	if cfg.Azure != nil && unredacted.Azure != nil {
		cfg.Azure.UnredactSecrets(unredacted.Azure)
	}
	if cfg.Swift != nil && unredacted.Swift != nil {
		cfg.Swift.UnredactSecrets(unredacted.Swift)
	}
}

func (cfg *S3StorageSpec) UnredactSecrets(unredacted *S3StorageSpec) {
	if cfg.SecretAccessKey == Redacted {
		cfg.SecretAccessKey = unredacted.SecretAccessKey
	}
	if cfg.Sse != nil {
		cfg.Sse.UnredactSecrets(unredacted.Sse)
	}
}

func (cfg *GCSStorageSpec) UnredactSecrets(unredacted *GCSStorageSpec) {
	if cfg.ServiceAccount == Redacted {
		cfg.ServiceAccount = unredacted.ServiceAccount
	}
}

func (cfg *AzureStorageSpec) UnredactSecrets(unredacted *AzureStorageSpec) {
	if cfg.StorageAccountKey == Redacted {
		cfg.StorageAccountKey = unredacted.StorageAccountKey
	}
	if cfg.MsiResource == Redacted {
		cfg.MsiResource = unredacted.MsiResource
	}
}

func (cfg *SwiftStorageSpec) UnredactSecrets(unredacted *SwiftStorageSpec) {
	if cfg.Password == Redacted {
		cfg.Password = unredacted.Password
	}
}

func (cfg *SSEConfig) UnredactSecrets(unredacted *SSEConfig) {
	if cfg.KmsEncryptionContext == Redacted {
		cfg.KmsEncryptionContext = unredacted.KmsEncryptionContext
	}
}
