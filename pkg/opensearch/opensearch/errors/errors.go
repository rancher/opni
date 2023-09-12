package errors

import "errors"

var (
	ErrConfigMissing        = errors.New("config value is missing")
	ErrSnapshotAlreadyExsts = errors.New("snapshot already exists")
	ErrNotFound             = errors.New("object not found")
)
