package ident

import (
	"context"
	"os"
	"strings"

	"github.com/google/uuid"
)

type hostPathProvider struct {
	path string
}

// Returns an ident.Provider that reads a UUID from a file if it exists. If
// the file does not exist, it will first be created with a new random UUID.
func NewHostPathProvider(path string) Provider {
	return &hostPathProvider{
		path: path,
	}
}

func (p *hostPathProvider) UniqueIdentifier(ctx context.Context) (string, error) {
	if _, err := os.Stat(p.path); err == nil {
		// File exists
		data, err := os.ReadFile(p.path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	newID := uuid.NewString()
	if err := os.WriteFile(p.path, []byte(newID), 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(p.path, 0600); err != nil {
		return "", err
	}
	return newID, nil
}
