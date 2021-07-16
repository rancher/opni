package errors

import errs "errors"

var (
	InvalidReference    = errs.New("referenced OpniCluster could not be found")
	UnsupportedProvider = errs.New("unsupported provider")
)
