package opensearchdata

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
)

const (
	initialAdminKey     = "initial-admin"
	initialAdminPending = "pending"
	initialAdminCreated = "created"
)

func (m *Manager) CreateInitialAdmin(password []byte, readyFunc ...ReadyFunc) {
	m.WaitForInit()
	m.kv.BackgroundInitClient(m.setJetStream)
	m.kv.WaitForInit()

	m.adminInitStateRW.Lock()
	_, err := m.kv.Client.PutString(initialAdminKey, initialAdminPending)
	if err != nil {
		m.logger.Warnf("failed to store initial admin state: %v", err)
	}
	m.adminInitStateRW.Unlock()

	for _, r := range readyFunc {
		exitEarly := r()
		if exitEarly {
			m.logger.Warn("opensearch cluster is never able to receive queries")
			return
		}

	}

	m.Lock()
	defer m.Unlock()

	ctx := context.Background()

	user := opensearchtypes.UserSpec{
		UserName: "opni",
		Password: string(password),
		BackendRoles: []string{
			"admin",
		},
	}

	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(5*time.Second),
		backoff.WithMaxInterval(1*time.Minute),
		backoff.WithMultiplier(1.1),
	)
	b := expBackoff.Start(ctx)
CREATE:
	for {
		select {
		case <-b.Done():
			m.logger.Warn("context cancelled before admin user created")
			return
		case <-b.Next():
			err := m.maybeCreateUser(ctx, user)
			if err != nil {
				m.logger.Errorf("failed to create admin user: %v", err)
				continue
			}
			break CREATE
		}
	}

	m.adminInitStateRW.Lock()
	_, err = m.kv.Client.PutString(initialAdminKey, initialAdminCreated)
	if err != nil {
		m.logger.Warnf("failed to store initial admin state: %v", err)
	}
	m.adminInitStateRW.Unlock()
}

func (m *Manager) userExists(ctx context.Context, name string) (bool, error) {
	resp, err := m.Security.GetUser(ctx, name)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil
	} else if resp.IsError() {
		return false, fmt.Errorf("response from API is %s", resp.String())
	}

	return true, nil
}

func (m *Manager) maybeCreateUser(ctx context.Context, user opensearchtypes.UserSpec) error {
	m.logger.Debug("creating opensearch admin user")
	exists, err := m.userExists(ctx, user.UserName)
	if err != nil {
		return err
	}
	if exists {
		m.logger.Debug("user already exists, doing nothing")
		return nil
	}

	resp, err := m.Security.CreateUser(ctx, user.UserName, opensearchutil.NewJSONReader(user))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("failed to create user: %s", resp.String())
	}
	m.logger.Debugf("user successfully created: %s", resp.String())
	return nil
}

func (m *Manager) ShouldCreateInitialAdmin() bool {
	m.kv.BackgroundInitClient(m.setJetStream)
	m.kv.WaitForInit()

	m.adminInitStateRW.RLock()
	defer m.adminInitStateRW.RUnlock()

	idExists, err := m.keyExists(initialAdminKey)
	if err != nil {
		m.logger.Errorf("failed to check initial admin state: %v", err)
		return false
	}

	if !idExists {
		m.logger.Debug("user creation not started, will install")
		return true
	}

	adminState, err := m.kv.Client.Get(initialAdminKey)
	if err != nil {
		m.logger.Errorf("failed to check initial admin state: %v", err)
		return false
	}

	switch string(adminState.Value()) {
	case initialAdminPending:
		m.logger.Debug("admin user creation is pending, restarting")
		return true
	case initialAdminCreated:
		m.logger.Debug("admin user already created, not restarting")
		return false
	default:
		m.logger.Error("invalid initial admin state returned")
		return false
	}
}

func (m *Manager) DeleteInitialAdminState() error {
	m.kv.BackgroundInitClient(m.setJetStream)
	m.kv.WaitForInit()
	m.adminInitStateRW.Lock()
	defer m.adminInitStateRW.Unlock()

	return m.kv.Client.Delete(initialAdminKey)
}
