package opensearchdata

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
)

func (m *Manager) CreateInitialAdmin(password []byte) {
	m.WaitForInit()
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
