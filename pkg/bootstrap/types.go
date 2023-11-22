package bootstrap

import (
	"context"
	"errors"

	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
)

type Bootstrapper interface {
	Bootstrap(context.Context, ident.Provider) (keyring.Keyring, error)
}

type Storage interface {
	storage.TokenStore
	storage.ClusterStore
	storage.KeyringStoreBroker
	storage.LockManagerBroker
}

type StorageConfig struct {
	storage.TokenStore
	storage.ClusterStore
	storage.KeyringStoreBroker
	storage.LockManagerBroker
}

func NewStorage(backend storage.Backend) Storage {
	lmb, ok := backend.(storage.LockManagerBroker)
	if !ok {
		lmb = inmemory.NewLockManagerBroker()
	}
	return StorageConfig{
		TokenStore:         backend,
		ClusterStore:       backend,
		KeyringStoreBroker: backend,
		LockManagerBroker:  lmb,
	}
}

var (
	ErrInvalidEndpoint    = errors.New("invalid endpoint")
	ErrNoRootCA           = errors.New("no root CA found in peer certificates")
	ErrLeafNotSigned      = errors.New("leaf certificate not signed by the root CA")
	ErrKeyExpired         = errors.New("key expired")
	ErrRootCAHashMismatch = errors.New("root CA hash mismatch")
	ErrNoValidSignature   = errors.New("no valid signature found in response")
	ErrNoToken            = errors.New("no bootstrap token provided")
)
