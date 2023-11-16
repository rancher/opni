package localauth

import (
	"context"

	"github.com/rancher/opni/pkg/auth/local"
)

const testPassword = "test"

type TestLocalAuthenticator struct{}

func (a *TestLocalAuthenticator) GenerateAdminPassword(_ context.Context) (string, error) {
	return testPassword, nil
}

func (a *TestLocalAuthenticator) ComparePassword(_ context.Context, pw []byte) error {
	if string(pw) != testPassword {
		return local.ErrInvalidPassword
	}
	return nil
}
