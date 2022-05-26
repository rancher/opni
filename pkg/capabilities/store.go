package capabilities

import (
	"fmt"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"go.uber.org/zap"
)

var (
	ErrBackendNotFound      = fmt.Errorf("backend not found")
	ErrBackendAlreadyExists = fmt.Errorf("backend already exists")
)

type BackendStore interface {
	Get(name string) (capability.Backend, error)
	Add(name string, backend capability.Backend) error
	List() []string
	RenderInstaller(name string, spec UserInstallerTemplateSpec) (string, error)
	CanInstall(capabilities ...string) error
	InstallCapabilities(cluster *corev1.Reference, capabilities ...string)
	UninstallCapabilities(cluster *corev1.Reference, capabilities ...string) error
}

type backendStore struct {
	serverSpec ServerInstallerTemplateSpec
	mu         sync.RWMutex
	backends   map[string]capability.Backend
	logger     *zap.SugaredLogger
}

func NewBackendStore(serverSpec ServerInstallerTemplateSpec, logger *zap.SugaredLogger) BackendStore {
	return &backendStore{
		serverSpec: serverSpec,
		backends:   make(map[string]capability.Backend),
		logger:     logger,
	}
}

func (s *backendStore) Get(name string) (capability.Backend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if backend, ok := s.backends[name]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrBackendNotFound, name)
	} else {
		return backend, nil
	}
}

func (s *backendStore) Add(name string, backend capability.Backend) error {
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

func (s *backendStore) RenderInstaller(name string, spec UserInstallerTemplateSpec) (string, error) {
	backend, err := s.Get(name)
	if err != nil {
		return "", err
	}
	return RenderInstallerCommand(backend.InstallerTemplate(), InstallerTemplateSpec{
		UserInstallerTemplateSpec:   spec,
		ServerInstallerTemplateSpec: s.serverSpec,
	})
}

func (s *backendStore) CanInstall(capabilities ...string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, capability := range capabilities {
		lg := s.logger.With(
			"capability", capability,
		)
		lg.Info("checking if capability can be installed")
		if b, ok := s.backends[capability]; !ok {
			lg.With(
				zap.Error(ErrUnknownCapability),
			).Error("cannot install capability")
			return fmt.Errorf("cannot install capability %s: %w", capability, ErrUnknownCapability)
		} else {
			if err := b.CanInstall(); err != nil {
				lg.With(
					zap.Error(err),
				).Error("cannot install capability")
				return fmt.Errorf("cannot install capability %q: %w", capability, err)
			}
			lg.Info("capability can be installed")
		}
	}
	return nil
}

func (s *backendStore) InstallCapabilities(
	cluster *corev1.Reference,
	capabilities ...string,
) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lg := s.logger.With(
		"cluster", cluster.GetId(),
	)
	lg.With(
		"capabilities", capabilities,
	).Info("installing capabilities for cluster")

	for _, capability := range capabilities {
		backend := s.backends[capability]
		// an installation can fail, but it is a fatal error. It is assumed that
		// CanInstall() has already been called and did not return an error.
		err := backend.Install(cluster)
		if err != nil {
			lg.With(
				"capability", capability,
				"error", err,
			).Fatal("failed to install capability")
		}
	}
}

func (s *backendStore) UninstallCapabilities(
	cluster *corev1.Reference,
	capabilities ...string,
) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lg := s.logger.With(
		"cluster", cluster.GetId(),
	)
	lg.With(
		"capabilities", capabilities,
	).Info("uninstalling capabilities for cluster")
	for _, capability := range capabilities {
		backend := s.backends[capability]
		err := backend.Uninstall(cluster)
		if err != nil {
			lg.With(
				"capability", capability,
				"error", err,
			).Error("failed to uninstall capability")
			return err
		}
	}
	return nil
}
