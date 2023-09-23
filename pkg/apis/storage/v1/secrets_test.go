package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
)

var _ = Describe("Secrets Redaction", Label("unit"), func() {
	It("should redact secrets", func() {
		cfg := &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: lo.ToPtr("secret"),
			},
			Azure: &storagev1.AzureConfig{
				AccountKey:  lo.ToPtr("secret"),
				MsiResource: lo.ToPtr("secret"),
			},
			Swift: &storagev1.SwiftConfig{
				Password: lo.ToPtr("secret"),
			},
		}
		cfg.RedactSecrets()

		Expect(cfg.S3.GetSecretAccessKey()).To(Equal(storagev1.Redacted))
		Expect(cfg.S3.Sse.GetKmsEncryptionContext()).To(Equal(storagev1.Redacted))
		Expect(cfg.Gcs.GetServiceAccount()).To(Equal(storagev1.Redacted))
		Expect(cfg.Azure.GetAccountKey()).To(Equal(storagev1.Redacted))
		Expect(cfg.Azure.GetMsiResource()).To(Equal(storagev1.Redacted))
		Expect(cfg.Swift.GetPassword()).To(Equal(storagev1.Redacted))

		cfg2 := (*storagev1.Config)(nil)
		Expect(cfg2.RedactSecrets).NotTo(Panic())
	})
	It("should un-redact secrets if previous values exist", func() {
		cfg := &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: lo.ToPtr("secret"),
			},
			Azure: &storagev1.AzureConfig{
				AccountKey:  lo.ToPtr("secret"),
				MsiResource: lo.ToPtr("secret"),
			},
			Swift: &storagev1.SwiftConfig{
				Password: lo.ToPtr("secret"),
			},
		}
		previous := cfg.DeepCopy()
		cfg.RedactSecrets()
		Expect(cfg.UnredactSecrets(previous)).To(Succeed())

		Expect(cfg.S3.GetSecretAccessKey()).To(Equal(previous.S3.GetSecretAccessKey()))
		Expect(cfg.S3.Sse.GetKmsEncryptionContext()).To(Equal(previous.S3.Sse.GetKmsEncryptionContext()))
		Expect(cfg.Gcs.GetServiceAccount()).To(Equal(previous.Gcs.GetServiceAccount()))
		Expect(cfg.Azure.GetAccountKey()).To(Equal(previous.Azure.GetAccountKey()))
		Expect(cfg.Azure.GetMsiResource()).To(Equal(previous.Azure.GetMsiResource()))
		Expect(cfg.Swift.GetPassword()).To(Equal(previous.Swift.GetPassword()))
	})
	It("should error when un-redacting if previous values do not exist", func() {
		previous := &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: lo.ToPtr("secret"),
			},
			Azure: &storagev1.AzureConfig{
				AccountKey:  lo.ToPtr("secret"),
				MsiResource: lo.ToPtr("secret"),
			},
			Swift: &storagev1.SwiftConfig{
				Password: lo.ToPtr("secret"),
			},
		}
		previous.RedactSecrets()
		Expect(previous.UnredactSecrets(nil)).NotTo(Succeed())
		Expect((*storagev1.Config)(nil).UnredactSecrets(previous)).To(Succeed())

		updated := &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: nil,
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: nil,
				},
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: nil,
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: lo.ToPtr("secret"),
			},
			Azure: &storagev1.AzureConfig{
				AccountKey: nil,
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: lo.ToPtr("secret"),
			},
			Azure: &storagev1.AzureConfig{
				AccountKey:  lo.ToPtr("secret"),
				MsiResource: nil,
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.Config{
			S3: &storagev1.S3Config{
				SecretAccessKey: lo.ToPtr("secret"),
				Sse: &storagev1.S3SSEConfig{
					KmsEncryptionContext: lo.ToPtr("secret"),
				},
			},
			Gcs: &storagev1.GcsConfig{
				ServiceAccount: lo.ToPtr("secret"),
			},
			Azure: &storagev1.AzureConfig{
				AccountKey:  lo.ToPtr("secret"),
				MsiResource: lo.ToPtr("secret"),
			},
			Swift: &storagev1.SwiftConfig{
				Password: nil,
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())
	})
})
