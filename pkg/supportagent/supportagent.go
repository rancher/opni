package supportagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/ident/identserver"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"go.uber.org/zap"
	"sigs.k8s.io/yaml"
)

const (
	dirPermissions  = os.FileMode(0700)
	filePermissions = os.FileMode(0600)
)

func MustLoadConfig(configFile string, lg logger.ExtendedSugaredLogger) *v1beta1.SupportAgentConfig {
	if configFile == "" {
		// find config file
		path, err := config.FindSupportConfig()
		switch {
		case err == nil:
			lg.With(
				"path", path,
			).Info("using config file")
			configFile = path
		case errors.Is(err, config.ErrConfigNotFound):
			wd, _ := os.Getwd()
			lg.Fatalf(`could not find a config file in ["%s", "$home/.opni], and --config was not given`, wd)
		default:
			lg.With(
				zap.Error(err),
			).Fatal("an error occurred while searching for a config file")
		}
	}

	agentConfig := &v1beta1.SupportAgentConfig{}
	if configFile != "" {
		objects, err := config.LoadObjectsFromFile(configFile)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Fatal("failed to load config")
		}
		if ok := objects.Visit(func(config *v1beta1.SupportAgentConfig) {
			agentConfig = config
		}); !ok {
			lg.Fatal("no support agent config found in config file")
		}
	} else {
		agentConfig.TypeMeta = v1beta1.SupportAgentConfigTypeMeta
	}

	return agentConfig
}

func PersistConfig(configFile string, config *v1beta1.SupportAgentConfig) error {
	if config == nil {
		return ErrNoConfig
	}
	if configFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		configFile = filepath.Join(home, ".opni", "support.yaml")
		ensureDirExists(configFile)
	}
	content, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(configFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, filePermissions)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(content)
	if err != nil {
		return err
	}

	return f.Sync()
}

func GatewayClientFromConfig(ctx context.Context, config *v1beta1.SupportAgentConfig) (clients.GatewayClient, error) {
	if config == nil {
		return nil, ErrNoConfig
	}
	kr, err := keyring.Unmarshal(config.Spec.AuthData.KeyringData)
	if err != nil {
		return nil, err
	}

	trust, err := machinery.BuildTrustStrategy(config.Spec.AuthData.TrustStrategy, kr)
	if err != nil {
		return nil, err
	}

	ipBuilder, err := ident.GetProviderBuilder("supportagent")
	if err != nil {
		return nil, err
	}
	ip := ipBuilder(config)

	client, err := clients.NewGatewayClient(ctx,
		config.Spec.GatewayAddress, ip, kr, trust)
	if err != nil {
		return nil, err
	}

	controlv1.RegisterIdentityServer(client, identserver.NewFromProvider(ip))
	return client, nil
}

func ensureDirExists(path string) error {
	return os.MkdirAll(filepath.Dir(path), os.ModeDir|dirPermissions)
}
