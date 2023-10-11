package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"log/slog"

	keystore "github.com/99designs/keyring"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/ident/identserver"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"sigs.k8s.io/yaml"
)

const (
	dirPermissions  = os.FileMode(0700)
	filePermissions = os.FileMode(0600)

	serviceName = "opni-support"
	keystoreKey = "keyring"
	defaultFile = "keyring"
)

func MustLoadConfig(configFile string, lg *slog.Logger) *v1beta1.SupportAgentConfig {
	if configFile == "" {
		// find config file
		path, err := config.FindSupportConfig()
		switch {
		case err == nil:
			lg.Info(

				"using config file", "path", path)

			configFile = path
		case errors.Is(err, config.ErrConfigNotFound):
			wd, _ := os.Getwd()
			lg.Error(fmt.Sprintf(`could not find a config file in ["%s", "$home/.opni], and --config was not given`, wd))
			os.Exit(1)
		default:
			lg.With(
				logger.Err(err),
			).Error("an error occurred while searching for a config file")
			os.Exit(1)
		}
	}

	agentConfig := &v1beta1.SupportAgentConfig{}
	if configFile != "" {
		objects, err := config.LoadObjectsFromFile(configFile)
		if err != nil {
			lg.With(
				logger.Err(err),
			).Error("failed to load config")
			os.Exit(1)
		}
		if ok := objects.Visit(func(config *v1beta1.SupportAgentConfig) {
			agentConfig = config
		}); !ok {
			lg.Error("no support agent config found in config file")
			os.Exit(1)
		}
	} else {
		agentConfig.TypeMeta = v1beta1.SupportAgentConfigTypeMeta
	}

	return agentConfig
}

func PersistConfig(
	configFile string,
	config *v1beta1.SupportAgentConfig,
	data []byte,
	storePwdFunc keystore.PromptFunc,
) error {
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

	err := storeKeyRingData(data, storePwdFunc)
	if err != nil {
		return err
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

func GatewayClientFromConfig(
	ctx context.Context,
	config *v1beta1.SupportAgentConfig,
	pwdFunc keystore.PromptFunc,
) (clients.GatewayClient, error) {
	if config == nil {
		return nil, ErrNoConfig
	}

	kr, err := LoadKeyring(pwdFunc)
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

func LoadKeyring(pwdFunc keystore.PromptFunc) (keyring.Keyring, error) {
	krData, err := openKeyRingData(pwdFunc)
	if err != nil {
		return nil, err
	}

	return keyring.Unmarshal(krData)
}

func ensureDirExists(path string) error {
	return os.MkdirAll(filepath.Dir(path), os.ModeDir|dirPermissions)
}

func openKeyRingData(pwdFunc keystore.PromptFunc) ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return []byte{}, err
	}

	keyringFile := filepath.Join(home, ".opni")

	config := keystore.Config{
		AllowedBackends: []keystore.BackendType{
			keystore.FileBackend,
		},
		ServiceName:      serviceName,
		FileDir:          keyringFile,
		FilePasswordFunc: pwdFunc,
	}

	kr, err := keystore.Open(config)
	if err != nil {
		return []byte{}, err
	}

	item, err := kr.Get(keystoreKey)
	if err != nil {
		return []byte{}, err
	}

	return item.Data, nil
}

func storeKeyRingData(data []byte, storePwdFunc keystore.PromptFunc) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	keyringFile := filepath.Join(home, ".opni")
	ensureDirExists(keyringFile)

	config := keystore.Config{
		AllowedBackends: []keystore.BackendType{
			keystore.FileBackend,
		},
		ServiceName:      serviceName,
		FileDir:          keyringFile,
		FilePasswordFunc: storePwdFunc,
	}

	kr, err := keystore.Open(config)
	if err != nil {
		return err
	}

	keyringItem := keystore.Item{
		Key:  keystoreKey,
		Data: data,
	}
	return kr.Set(keyringItem)
}
