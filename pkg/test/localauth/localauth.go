package localauth

import (
	"context"
	"errors"
)

const testPassword = "test"

type TestLocalAuthenticator struct{}

func (a *TestLocalAuthenticator) GenerateAdminPassword(_ context.Context) (string, error) {
	return testPassword, nil
}

func (a *TestLocalAuthenticator) ComparePassword(_ context.Context, pw []byte) error {
	if string(pw) != testPassword {
		return errors.New("mismatched password")
	}
	return nil
}
