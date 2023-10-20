package management

import (
	"context"
	"encoding/json"
	"sync"

	"log/slog"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type DashboardSettingsManager struct {
	lock   sync.RWMutex
	kv     storage.KeyValueStore
	logger *slog.Logger
}

func (m *DashboardSettingsManager) GetDashboardSettings(
	ctx context.Context,
	_ *emptypb.Empty,
) (*managementv1.DashboardSettings, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	settings := &managementv1.DashboardSettings{
		Global: &managementv1.DashboardGlobalSettings{},
		User:   map[string]string{},
	}

	if globalSettings, err := m.kv.Get(ctx, "settings/global"); err == nil {
		if err := proto.Unmarshal(globalSettings, settings.Global); err != nil {
			// ignore errors
			m.logger.With(
				zap.Error(err),
			).Warn("failed to unmarshal global settings")
		}
	}

	if userSettings, err := m.kv.Get(ctx, "settings/user"); err == nil {
		if err := json.Unmarshal(userSettings, &settings.User); err != nil {
			// ignore errors
			m.logger.With(
				zap.Error(err),
			).Warn("failed to unmarshal user settings")
		}
	}

	return settings, nil
}

func (m *DashboardSettingsManager) UpdateDashboardSettings(
	ctx context.Context,
	settings *managementv1.DashboardSettings,
) (*emptypb.Empty, error) {
	current, err := m.GetDashboardSettings(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if settings.Global != nil {
		current.Global = settings.Global
	}

	for k, v := range settings.User {
		if v == "" || v == "-" {
			delete(current.User, k)
		} else {
			current.User[k] = v
		}
	}

	globalSettings, err := proto.Marshal(current.Global)
	if err != nil {
		return nil, err
	}
	userSettings, err := json.Marshal(current.User)
	if err != nil {
		return nil, err
	}

	if err := m.kv.Put(ctx, "settings/global", globalSettings); err != nil {
		return nil, err
	}
	if err := m.kv.Put(ctx, "settings/user", userSettings); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
