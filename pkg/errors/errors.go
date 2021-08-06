package errors

import errs "errors"

var (
	InvalidReference    = errs.New("referenced OpniCluster could not be found")
	UnsupportedProvider = errs.New("unsupported provider")
	ErrS3Credentials    = errs.New("could not configure external s3 credentials")
)
