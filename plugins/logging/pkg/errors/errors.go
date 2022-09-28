package errors

import (
	"errors"
	"fmt"
)

var (
	ErrClusterAlreadyExists    = errors.New("cluster already exists")
	ErrInvalidList             = errors.New("list did not return exactly 1 result")
	ErrInvalidPersistence      = errors.New("invalid persistence config")
	ErrInvalidCluster          = errors.New("invalid opensearch cluster")
	ErrClusterIDMissing        = errors.New("request does not include cluster ID")
	ErrOpensearchResponse      = errors.New("opensearch request unsuccessful")
	ErrNoOpensearchClient      = errors.New("opensearch client is not set")
	ErrLoggingCapabilityExists = errors.New("at least one cluster has logging capability installed")
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

func ErrGetDetailsInvalidList(id string) error {
	return fmt.Errorf("fetching credentials for cluster %s failed: %w", id, ErrInvalidList)
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

func ErrStoredClusterPersistence() error {
	return fmt.Errorf("stored opensearch cluster is invalid: %w", ErrInvalidPersistence)
}

func ErrRequestMissingMemory() error {
	return fmt.Errorf("memory limit must be configured: %w", ErrInvalidCluster)
}

func ErrInvalidRetention() error {
	return fmt.Errorf("duration must be integer and time unit, e.g 7d: %w", ErrInvalidCluster)
}

func ErrOpensearchRequestFailed(status string) error {
	return fmt.Errorf("%s: %w", status, ErrOpensearchResponse)
}
