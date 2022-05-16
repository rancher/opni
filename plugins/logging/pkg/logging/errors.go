package logging

import (
	"errors"
	"fmt"
)

var (
	ErrClusterAlreadyExists = errors.New("cluster already exists")
	ErrInvalidList          = errors.New("list did not return exactly 1 result")
	ErrClusterIDMissing     = errors.New("request does not include cluster ID")
)

func ErrCreateNamespaceFailed(clienterr error) error {
	return fmt.Errorf("failed to create storage namespace: %w", clienterr)
}

func ErrCheckOpensearchClusterFailed(clienterr error) error {
	return fmt.Errorf("failed to check opensearch cluster exists: %w", clienterr)
}

func ErrListingClustersFaled(clienterr error) error {
	return fmt.Errorf("failed to list clusters: %w", clienterr)
}

func ErrCreateFailedAlreadyExists(id string) error {
	return fmt.Errorf("creating cluster %s failed: %w", id, ErrClusterAlreadyExists)
}

func ErrDeleteClusterInvalidList(id string) error {
	return fmt.Errorf("listing cluster %s failed: %w", id, ErrInvalidList)
}

func ErrGenerateCredentialsFailed(err error) error {
	return fmt.Errorf("failed to generate Opensearch credentials: %w", err)
}

func ErrStoreUserCredentialsFailed(err error) error {
	return fmt.Errorf("failed to store user credentials: %w", err)
}

func ErrStoreClusterFailed(err error) error {
	return fmt.Errorf("failed to store logging cluster: %w", err)
}
