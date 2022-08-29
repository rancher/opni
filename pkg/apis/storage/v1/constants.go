package v1

import (
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/bucket/s3"
)

const (
	S3         = bucket.S3
	GCS        = bucket.GCS
	Azure      = bucket.Azure
	Swift      = bucket.Swift
	Filesystem = bucket.Filesystem

	SignatureVersionV4 = s3.SignatureVersionV4
	SignatureVersionV2 = s3.SignatureVersionV2
	SSEKMS             = s3.SSEKMS
	SSES3              = s3.SSES3
)

var (
	supportedBackends          = []string{S3, GCS, Azure, Swift, Filesystem}
	supportedSignatureVersions = []string{SignatureVersionV4, SignatureVersionV2}
	supportedSSETypes          = []string{SSEKMS, SSES3}
)
