package gateway

import (
	"fmt"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/capability"
	"go.uber.org/zap"
)

type CapabilityBackendStore struct {
	backends map[string]capability.Backend
	logger   *zap.SugaredLogger
}

func NewCapabilityBackendStore(logger *zap.SugaredLogger) *CapabilityBackendStore {
	return &CapabilityBackendStore{
		backends: make(map[string]capability.Backend),
		logger:   logger,
	}
}

func (s *CapabilityBackendStore) Get(name string) (capability.Backend, error) {
	if backend, ok := s.backends[name]; !ok {
		return nil, fmt.Errorf("backend %s does not exist", name)
	} else {
		return backend, nil
	}
}

func (s *CapabilityBackendStore) Add(name string, backend capability.Backend) error {
	if _, ok := s.backends[name]; ok {
		return fmt.Errorf("backend for capability %s already exists", name)
	}
	s.backends[name] = backend
	return nil
}

func (s *CapabilityBackendStore) InstallCapabilities(
	cluster *core.Reference,
	capabilities []string,
) (installErr error) {
	lg := s.logger.With(
		"cluster", cluster.GetId(),
	)
	lg.With(
		"capabilities", capabilities,
	).Info("installing capabilities for cluster")

	// make sure all the backends exist before continuing, so we don't have to
	// revert anything if we don't need to
	for _, capability := range capabilities {
		if _, ok := s.backends[capability]; !ok {
			lg.With(
				"capability", capability,
			).Error("capability backend does not exist")
			return fmt.Errorf("backend for capability %s does not exist", capability)
		}
	}

	// attempt to install each capability. if any fail, roll back previous ones
	for _, capability := range capabilities {
		backend := s.backends[capability]
		err := backend.Install(cluster)
		if err != nil {
			lg.With(
				"capability", capability,
				"error", err,
			).Error("failed to install capability")
			return fmt.Errorf("failed to install capability %s: %v", capability, err)
		}
		defer func(capability string) {
			if installErr != nil {
				lg.With(
					"capability", capability,
				).Warn("reverting installation for capability")
				if err := backend.Uninstall(); err != nil {
					lg.With(
						"capability", capability,
					).Error("failed to revert installation for capability")
				}
			}
		}(capability)
	}
	lg.Info("successfully installed capabilities")
	return nil
}
