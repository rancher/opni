package supportagent

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/util"
)

type supportAgentProvider struct {
	supportAgentProviderOptions
	ephemeralID string
}

type supportAgentProviderOptions struct {
	configPath string
	config     *v1beta1.SupportAgentConfig
}

type SupportAgentProviderOption func(*supportAgentProviderOptions)

func (o *supportAgentProviderOptions) apply(opts ...SupportAgentProviderOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithConfigPath(path string) SupportAgentProviderOption {
	return func(o *supportAgentProviderOptions) {
		o.configPath = path
	}
}

func WithConfig(config *v1beta1.SupportAgentConfig) SupportAgentProviderOption {
	return func(o *supportAgentProviderOptions) {
		o.config = config
	}
}

func newSupportAgentProvider(opts ...SupportAgentProviderOption) ident.Provider {
	path, err := config.FindSupportConfig()
	if err != nil {
		if !errors.Is(err, config.ErrConfigNotFound) {
			panic(err)
		}
	}
	options := supportAgentProviderOptions{
		configPath: path,
	}
	options.apply(opts...)

	return &supportAgentProvider{
		supportAgentProviderOptions: options,
		ephemeralID:                 uuid.New().String(),
	}
}

func (p *supportAgentProvider) UniqueIdentifier(_ context.Context) (string, error) {
	if p.config != nil {
		if p.config.Spec.UserID != "" {
			return p.config.Spec.UserID, nil
		}
	}

	if p.configPath != "" {
		objects, err := config.LoadObjectsFromFile(p.configPath)
		if err != nil {
			return "", err
		}

		var agentConfig *v1beta1.SupportAgentConfig
		if ok := objects.Visit(func(config *v1beta1.SupportAgentConfig) {
			agentConfig = config
		}); !ok {
			return "", errors.New("no support agent config found in config file")
		}

		if agentConfig.Spec.UserID != "" {
			return agentConfig.Spec.UserID, nil
		}
	}

	return p.ephemeralID, nil
}

func init() {
	util.Must(ident.RegisterProvider("supportagent", func(args ...any) ident.Provider {
		var opts []SupportAgentProviderOption
		for _, arg := range args {
			switch v := arg.(type) {
			case string:
				opts = append(opts, WithConfigPath(v))
			case *v1beta1.SupportAgentConfig:
				opts = append(opts, WithConfig(v))
			default:
				panic(fmt.Sprintf("invalid argument type %T", v))
			}
		}
		return newSupportAgentProvider(opts...)
	}))
}
