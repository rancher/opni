package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	_ "github.com/rancher/opni/pkg/ident/supportagent"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/supportagent"
	supportagentconfig "github.com/rancher/opni/pkg/supportagent/config"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildSupportCmd() *cobra.Command {
	supportCmd := &cobra.Command{
		Use:     "support",
		Aliases: []string{"support-agent"},
		Short:   "Opni support agent",
	}

	supportCmd.AddCommand(BuildSupportBootstrapCmd())
	supportCmd.AddCommand(BuildSupportPingCmd())
	supportCmd.AddCommand(BuildSupportShipCmd())
	return supportCmd
}

func BuildSupportBootstrapCmd() *cobra.Command {
	var configFile, logLevel, token, endpoint string

	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap the support agent",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, ca := context.WithCancel(waitctx.FromContext(cmd.Context()))
			defer ca()

			agentlg := logger.New(logger.WithLogLevel(util.Must(zapcore.ParseLevel(logLevel))))

			if configFile == "" {
				// find config file
				path, err := config.FindSupportConfig()
				switch {
				case err == nil:
					agentlg.With(
						"path", path,
					).Info("using config file")
					configFile = path
				case errors.Is(err, config.ErrConfigNotFound):
					wd, _ := os.Getwd()
					agentlg.Infof(`could not find a config file in ["%s", "$home/.opni], and --config was not given`, wd)
				default:
					agentlg.With(
						zap.Error(err),
					).Fatal("an error occurred while searching for a config file")
				}
			}

			agentConfig := &v1beta1.SupportAgentConfig{}
			if configFile != "" {
				objects, err := config.LoadObjectsFromFile(configFile)
				if err != nil {
					agentlg.With(
						zap.Error(err),
					).Fatal("failed to load config")
				}
				if ok := objects.Visit(func(config *v1beta1.SupportAgentConfig) {
					agentConfig = config
				}); !ok {
					agentlg.Fatal("no support agent config found in config file")
				}
			} else {
				agentConfig.TypeMeta = v1beta1.SupportAgentConfigTypeMeta
			}

			pins := cmd.Flags().Lookup("pin").Value.(pflag.SliceValue)
			if len(pins.GetSlice()) == 0 {
				pins.Replace(agentConfig.Spec.AuthData.Pins)
			}

			strategy := cmd.Flags().Lookup("trust-strategy").Value
			if strategy.String() == "" {
				strategy.Set(string(agentConfig.Spec.AuthData.TrustStrategy))
			} else {
				agentConfig.Spec.AuthData.TrustStrategy = v1beta1.TrustStrategyKind(strategy.String())
			}

			switch {
			case token != "":
			case agentConfig.Spec.AuthData.Token != "":
				token = agentConfig.Spec.AuthData.Token
			default:
				agentlg.Fatal("no token provided")
			}

			bootstrapper, err := configureSupportAgentBootstrap(
				cmd.Flags(),
				token,
				endpoint,
				agentlg,
			)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to configure bootstrap")
			}

			ipBuilder, err := ident.GetProviderBuilder("supportagent")
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to get ident provider")
			}
			ip := ipBuilder(agentConfig)

			userid, err := ip.UniqueIdentifier(ctx)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to get unique identifier")
			}

			kr, err := bootstrapper.Bootstrap(ctx, ip)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to bootstrap")
			}

			keyringData, err := kr.Marshal()
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to marshal keyring")
			}

			agentConfig.Spec.UserID = userid
			agentConfig.Spec.GatewayAddress = endpoint
			agentConfig.Spec.AuthData.Token = ""

			err = supportagentconfig.PersistConfig(configFile, agentConfig, keyringData, getStorePassword)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to persist config")
			}
		},
	}

	trust.BindFlags(bootstrapCmd.Flags())
	bootstrapCmd.Flags().StringVar(&configFile, "config", "", "path to config file")
	bootstrapCmd.Flags().StringVar(&logLevel, "log-level", "info", "log level")
	bootstrapCmd.Flags().StringVar(&token, "token", "", "token to use for bootstrap")
	bootstrapCmd.Flags().StringVar(&endpoint, "endpoint", "", "gateway endpoint to use for bootstrap")

	return bootstrapCmd
}

func BuildSupportPingCmd() *cobra.Command {
	var configFile, logLevel string

	pingCmd := &cobra.Command{
		Use:   "ping",
		Short: "Ping the gateway",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, ca := context.WithCancel(waitctx.FromContext(cmd.Context()))
			defer ca()

			agentlg := logger.New(logger.WithLogLevel(util.Must(zapcore.ParseLevel(logLevel))))

			config := supportagentconfig.MustLoadConfig(configFile, agentlg)

			gatewayClient, err := supportagentconfig.GatewayClientFromConfig(ctx, config, getRetrievePassword)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to get gateway client")
			}

			cc, futureErr := gatewayClient.Connect(ctx)
			if futureErr.IsSet() {
				agentlg.With(
					zap.Error(futureErr.Get()),
				).Fatal("failed to connect to gateway")
			}
			pingClient := corev1.NewPingerClient(cc)
			resp, err := pingClient.Ping(ctx, &emptypb.Empty{})
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to ping gateway")
			}
			agentlg.Info(resp.Message)
		},
	}

	pingCmd.Flags().StringVar(&configFile, "config", "", "path to config file")
	pingCmd.Flags().StringVar(&logLevel, "log-level", "info", "log level")

	return pingCmd
}

func BuildSupportShipCmd() *cobra.Command {
	const (
		nodeNameKey   = "node_name"
		caseNumberKey = "case_number"
	)
	var configFile, logLevel, caseNumber, nodeName string

	shipCmd := &cobra.Command{
		Use:   "ship",
		Short: "Ship support logs to Opni",
		ValidArgs: []string{
			string(RKE),
			string(K3S),
			string(RKE2),
		},
		PreRunE: validateShipArgs,
		Run: func(cmd *cobra.Command, args []string) {
			ctx, ca := context.WithCancel(waitctx.FromContext(cmd.Context()))
			defer ca()

			agentlg := logger.New(logger.WithLogLevel(util.Must(zapcore.ParseLevel(logLevel))))

			config := supportagentconfig.MustLoadConfig(configFile, agentlg)

			gatewayClient, err := supportagentconfig.GatewayClientFromConfig(ctx, config, getRetrievePassword)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to get gateway client")
			}

			cc, futureErr := gatewayClient.Connect(ctx)
			if futureErr.IsSet() {
				agentlg.With(
					zap.Error(futureErr.Get()),
				).Fatal("failed to connect to gateway")
			}

			if cc == nil {
				agentlg.With(
					zap.Error(futureErr.Get()),
				).Fatal("failed to connect to gateway")
			}

			md := metadata.New(map[string]string{
				supportagent.AttributeValuesKey: "",
			})
			md.Set(supportagent.AttributeValuesKey,
				caseNumberKey, caseNumber,
				nodeNameKey, nodeName,
			)

			ctx = metadata.NewOutgoingContext(ctx, md)

			switch Distribution(args[0]) {
			case RKE:
				shipRKELogs(ctx, cc, agentlg.Zap())
			case K3S:
				shipK3sLogs(ctx, cc, agentlg.Zap())
			case RKE2:
				shipRKE2Logs(ctx, cc, agentlg.Zap())
			default:
				agentlg.Error("invalid cluster type, must be one of rke, k3s, or rke2")
			}
		},
	}

	shipCmd.Flags().StringVar(&configFile, "config", "", "path to config file")
	shipCmd.Flags().StringVar(&logLevel, "log-level", "info", "log level")
	shipCmd.Flags().StringVar(&caseNumber, "case-number", "", "case number to attach to logs")
	shipCmd.Flags().StringVar(&nodeName, "node-name", "", "node name to attach to logs")

	return shipCmd
}

func validateShipArgs(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("must specify a cluster type")
	}
	return nil
}

func configureSupportAgentBootstrap(
	flags *pflag.FlagSet,
	tokenData string,
	endpoint string,
	agentlg logger.ExtendedSugaredLogger,
) (bootstrap.Bootstrapper, error) {
	strategyConfig, err := trust.BuildConfigFromFlags(flags)
	if err != nil {
		return nil, err
	}
	strategy, err := strategyConfig.Build()
	if err != nil {
		return nil, err
	}

	token, err := tokens.ParseHex(tokenData)
	if err != nil {
		agentlg.With(
			zap.Error(err),
			zap.String("token", fmt.Sprintf("[redacted (len: %d)]", len(tokenData))),
		).Error("failed to parse token")
		return nil, err
	}

	return &bootstrap.ClientConfigV2{
		Token:         token,
		TrustStrategy: strategy,
		Endpoint:      endpoint,
	}, nil
}

func getStorePassword(_ string) (string, error) {
	password := ""
	prompt := &survey.Password{
		Message: chalk.Yellow.Color("Please enter the password to store the keyring with"),
	}
	err := survey.AskOne(
		prompt,
		&password,
		survey.WithValidator(survey.Required),
	)
	if err != nil {
		return "", err
	}
	return password, nil
}

func getRetrievePassword(_ string) (string, error) {
	password := ""
	prompt := &survey.Password{
		Message: chalk.Yellow.Color("Please enter the password to fetch the keyring with"),
	}
	err := survey.AskOne(
		prompt,
		&password,
		survey.WithValidator(survey.Required),
	)
	if err != nil {
		return "", err
	}
	return password, nil
}

func init() {
	AddCommandsToGroup(Utilities, BuildSupportCmd())
}
