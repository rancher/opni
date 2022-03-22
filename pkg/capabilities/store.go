package capabilities

import (
	"fmt"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/capability"
	"go.uber.org/zap"
)

type BackendStore interface {
	Get(name string) (capability.Backend, error)
	Add(name string, backend capability.Backend) error
	List() []string
	RenderInstaller(name string, spec InstallerTemplateSpec) (string, error)
	CanInstall(capabilities ...string) error
	InstallCapabilities(cluster *core.Reference, capabilities ...string)
}

type backendStore struct {
	backends map[string]capability.Backend
	logger   *zap.SugaredLogger
}

func NewBackendStore(logger *zap.SugaredLogger) BackendStore {
	return &backendStore{
		backends: make(map[string]capability.Backend),
		logger:   logger,
	}
}

func (s *backendStore) Get(name string) (capability.Backend, error) {
	if backend, ok := s.backends[name]; !ok {
		return nil, fmt.Errorf("backend %q does not exist", name)
	} else {
		return backend, nil
	}
}

func (s *backendStore) Add(name string, backend capability.Backend) error {
	if _, ok := s.backends[name]; ok {
		return fmt.Errorf("backend for capability %q already exists", name)
	}
	s.backends[name] = backend
	return nil
}

func (s *backendStore) List() []string {
	var capabilities []string
	for capability := range s.backends {
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

func (s *backendStore) RenderInstaller(name string, spec InstallerTemplateSpec) (string, error) {
	backend, err := s.Get(name)
	if err != nil {
		return "", err
	}
	return RenderInstallerCommand(backend.InstallerTemplate(), spec)
}

func (s *backendStore) CanInstall(capabilities ...string) error {
	for _, capability := range capabilities {
		if b, ok := s.backends[capability]; !ok {
			return fmt.Errorf("backend for capability %q does not exist", capability)
		} else {
			if err := b.CanInstall(); err != nil {
				return fmt.Errorf("cannot install capability %q: %w", capability, err)
			}
		}
	}
	return nil
}

func (s *backendStore) InstallCapabilities(
	cluster *core.Reference,
	capabilities ...string,
) {
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
