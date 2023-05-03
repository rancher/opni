package ident

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/pkg/util"
)

type envProvider struct {
	EnvIdentOptions
}

type EnvIdentOptions struct {
	variableName string
}

type EnvIdentOption func(*EnvIdentOptions)

func (o *EnvIdentOptions) apply(opts ...EnvIdentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithVariableName(name string) EnvIdentOption {
	return func(o *EnvIdentOptions) {
		o.variableName = name
	}
}

func NewEnvProvider(opts ...EnvIdentOption) Provider {
	options := EnvIdentOptions{
		variableName: "OPNI_UNIQUE_IDENTIFIER",
	}
	options.apply(opts...)
	return &envProvider{
		EnvIdentOptions: options,
	}
}

func (p *envProvider) UniqueIdentifier(_ context.Context) (string, error) {
	if v, ok := os.LookupEnv(p.variableName); ok {
		return v, nil
	}
	return "", fmt.Errorf("environment variable %s not set", p.variableName)
}

func init() {
	util.Must(RegisterProvider("env", func(...any) Provider {
		return NewEnvProvider()
	}))
}
