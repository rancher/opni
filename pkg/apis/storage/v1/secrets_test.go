package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
)

var _ = Describe("Secrets Redaction", func() {
	It("should redact secrets", func() {
		cfg := &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "secret",
			},
			Azure: &storagev1.AzureStorageSpec{
				StorageAccountKey: "secret",
				MsiResource:       "secret",
			},
			Swift: &storagev1.SwiftStorageSpec{
				Password: "secret",
			},
		}
		cfg.RedactSecrets()

		Expect(cfg.S3.SecretAccessKey).To(Equal(storagev1.Redacted))
		Expect(cfg.S3.Sse.KmsEncryptionContext).To(Equal(storagev1.Redacted))
		Expect(cfg.Gcs.ServiceAccount).To(Equal(storagev1.Redacted))
		Expect(cfg.Azure.StorageAccountKey).To(Equal(storagev1.Redacted))
		Expect(cfg.Azure.MsiResource).To(Equal(storagev1.Redacted))
		Expect(cfg.Swift.Password).To(Equal(storagev1.Redacted))

		cfg2 := (*storagev1.StorageSpec)(nil)
		Expect(cfg2.RedactSecrets).NotTo(Panic())
	})
	It("should un-redact secrets if previous values exist", func() {
		cfg := &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "secret",
			},
			Azure: &storagev1.AzureStorageSpec{
				StorageAccountKey: "secret",
				MsiResource:       "secret",
			},
			Swift: &storagev1.SwiftStorageSpec{
				Password: "secret",
			},
		}
		previous := cfg.DeepCopy()
		cfg.RedactSecrets()
		Expect(cfg.UnredactSecrets(previous)).To(Succeed())

		Expect(cfg.S3.SecretAccessKey).To(Equal(previous.S3.SecretAccessKey))
		Expect(cfg.S3.Sse.KmsEncryptionContext).To(Equal(previous.S3.Sse.KmsEncryptionContext))
		Expect(cfg.Gcs.ServiceAccount).To(Equal(previous.Gcs.ServiceAccount))
		Expect(cfg.Azure.StorageAccountKey).To(Equal(previous.Azure.StorageAccountKey))
		Expect(cfg.Azure.MsiResource).To(Equal(previous.Azure.MsiResource))
		Expect(cfg.Swift.Password).To(Equal(previous.Swift.Password))
	})
	It("should error when un-redacting if previous values do not exist", func() {
		previous := &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "secret",
			},
			Azure: &storagev1.AzureStorageSpec{
				StorageAccountKey: "secret",
				MsiResource:       "secret",
			},
			Swift: &storagev1.SwiftStorageSpec{
				Password: "secret",
			},
		}
		previous.RedactSecrets()
		Expect(previous.UnredactSecrets(nil)).NotTo(Succeed())
		Expect((*storagev1.StorageSpec)(nil).UnredactSecrets(previous)).To(Succeed())

		updated := &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "",
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "",
				},
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "",
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "secret",
			},
			Azure: &storagev1.AzureStorageSpec{
				StorageAccountKey: "",
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "secret",
			},
			Azure: &storagev1.AzureStorageSpec{
				StorageAccountKey: "secret",
				MsiResource:       "",
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())

		updated = &storagev1.StorageSpec{
			S3: &storagev1.S3StorageSpec{
				SecretAccessKey: "secret",
				Sse: &storagev1.SSEConfig{
					KmsEncryptionContext: "secret",
				},
			},
			Gcs: &storagev1.GCSStorageSpec{
				ServiceAccount: "secret",
			},
			Azure: &storagev1.AzureStorageSpec{
				StorageAccountKey: "secret",
				MsiResource:       "secret",
			},
			Swift: &storagev1.SwiftStorageSpec{
				Password: "",
			},
		}
		Expect(previous.UnredactSecrets(updated)).NotTo(Succeed())
	})
})
