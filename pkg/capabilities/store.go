package capabilities

import (
	"fmt"
	"sync"

	"log/slog"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
)

var (
	ErrBackendNotFound      = fmt.Errorf("capability backend not found")
	ErrBackendAlreadyExists = fmt.Errorf("capability backend already exists")
)

type BackendStore interface {
	// Obtain a backend client for the given capability name
	Get(name string) (capabilityv1.BackendClient, error)
	// Add a capability backend with the given name
	Add(name string, backend capabilityv1.BackendClient) error
	// Returns all capability names known to the store
	List() []string
}

type backendStore struct {
	capabilityv1.UnsafeBackendServer
	mu       sync.RWMutex
	backends map[string]capabilityv1.BackendClient
	logger   *slog.Logger
}

func NewBackendStore(logger *slog.Logger) BackendStore {
	return &backendStore{
		backends: make(map[string]capabilityv1.BackendClient),
		logger:   logger,
	}
}

func (s *backendStore) Get(name string) (capabilityv1.BackendClient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	backend, ok := s.backends[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrBackendNotFound, name)
	}
	return backend, nil
}

func (s *backendStore) Add(name string, backend capabilityv1.BackendClient) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.backends[name]; ok {
		return fmt.Errorf("%w: %s", ErrBackendAlreadyExists, name)
	}
	s.backends[name] = backend
	return nil
}

func (s *backendStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	capabilities := make([]string, 0, len(s.backends))
	for capability := range s.backends {
		capabilities = append(capabilities, capability)
	}
	return capabilities
}
