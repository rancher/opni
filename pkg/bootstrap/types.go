package bootstrap

import (
	"context"

	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
)

type Bootstrapper interface {
	Bootstrap(context.Context, ident.Provider) (keyring.Keyring, error)
	Finalize(context.Context) error
}
