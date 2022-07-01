package commands

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/events"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	configLocation       string
	agentLogLevel        string
	eventOutputEndpoint  string
	enableMetrics        bool
	enableLogging        bool
	enableEventCollector bool

	agentlg logger.ExtendedSugaredLogger
)

func BuildAgentCmd() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the Opni Monitoring Agent",
		Long: `The client component of the opni gateway, used to proxy the prometheus
agent remote-write requests to add dynamic authentication.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !enableMetrics && !enableLogging {
				return errors.New("at least one of [--metrics, --logging] must be set")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := waitctx.FromContext(cmd.Context())
			tracing.Configure("agent")
			agentlg = logger.New(logger.WithLogLevel(util.Must(zapcore.ParseLevel(agentLogLevel))))
			wg := sync.WaitGroup{}

			if enableMetrics {
				wg.Add(1)
				go func(ctx context.Context) {
					defer wg.Done()
					runMonitoringAgent(ctx)
				}(ctx)
			}

			if enableLogging {
				wg.Add(1)
				go func(ctx context.Context) {
					defer wg.Done()
					err := runLoggingControllers(ctx)
					if err != nil {
						agentlg.Fatalf("failed to start controllers: %v", err)
					}
				}(ctx)
			}

			if enableEventCollector {
				wg.Add(1)
				go func(ctx context.Context) {
					defer wg.Done()
					err := runEventsCollector(ctx)
					if err != nil {
						agentlg.Fatalf("failed to run event collector: %v", err)
					}
				}(cmd.Context())
			}

			wg.Wait()
		},
	}

	agentCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	agentCmd.Flags().StringVar(&agentLogLevel, "log-level", "info", "log level (debug, info, warning, error)")
	agentCmd.Flags().BoolVar(&enableMetrics, "metrics", false, "enable metrics agent")
	agentCmd.Flags().BoolVar(&enableLogging, "logging", false, "enable logging controllers")
	agentCmd.Flags().BoolVar(&enableEventCollector, "events", false, "enable event collector")
	agentCmd.Flags().StringVar(&eventOutputEndpoint, "events-output", "http://opni-shipper:2021/log/ingest", "endpoint to post events to")
	return agentCmd
}

func runMonitoringAgent(ctx context.Context) {
	if configLocation == "" {
		// find config file
		path, err := config.FindConfig()
		if err != nil {
			if errors.Is(err, config.ErrConfigNotFound) {
				wd, _ := os.Getwd()
				agentlg.Fatalf(`could not find a config file in ["%s","/etc/opni-monitoring"], and --config was not given`, wd)
			}
			agentlg.With(
				zap.Error(err),
			).Fatal("an error occurred while searching for a config file")
		}
		agentlg.With(
			"path", path,
		).Info("using config file")
		configLocation = path
	}

	objects, err := config.LoadObjectsFromFile(configLocation)
	if err != nil {
		agentlg.With(
			zap.Error(err),
		).Fatal("failed to load config")
	}
	var agentConfig *v1beta1.AgentConfig
	objects.Visit(func(config *v1beta1.AgentConfig) {
		agentConfig = config
	})

	var bootstrapper bootstrap.Bootstrapper
	var trustStrategy trust.Strategy
	if agentConfig.Spec.Bootstrap != nil {
		if agentConfig.Spec.Bootstrap.InClusterManagementAddress != nil {
			bootstrapper = &bootstrap.InClusterBootstrapper{
				Capability:         wellknown.CapabilityMetrics,
				GatewayEndpoint:    agentConfig.Spec.GatewayAddress,
				ManagementEndpoint: *agentConfig.Spec.Bootstrap.InClusterManagementAddress,
			}
		} else {
			agentlg.Info("loading bootstrap tokens from config file")
			tokenData := agentConfig.Spec.Bootstrap.Token

			switch agentConfig.Spec.TrustStrategy {
			case v1beta1.TrustStrategyPKP:
				pins := agentConfig.Spec.Bootstrap.Pins
				publicKeyPins := make([]*pkp.PublicKeyPin, len(pins))
				for i, pin := range pins {
					publicKeyPins[i], err = pkp.DecodePin(pin)
					if err != nil {
						agentlg.With(
							zap.Error(err),
							zap.String("pin", string(pin)),
						).Error("failed to parse pin")
					}
				}
				conf := trust.StrategyConfig{
					PKP: &trust.PKPConfig{
						Pins: trust.NewPinSource(publicKeyPins),
					},
				}
				trustStrategy, err = conf.Build()
				if err != nil {
					agentlg.With(
						zap.Error(err),
					).Fatal("error configuring PKP trust strategy")
				}
			case v1beta1.TrustStrategyCACerts:
				paths := agentConfig.Spec.Bootstrap.CACerts
				certs := []*x509.Certificate{}
				for _, path := range paths {
					data, err := os.ReadFile(path)
					if err != nil {
						agentlg.With(
							zap.Error(err),
							zap.String("path", path),
						).Fatal("failed to read CA cert")
					}
					cert, err := util.ParsePEMEncodedCert(data)
					if err != nil {
						agentlg.With(
							zap.Error(err),
							zap.String("path", path),
						).Fatal("failed to parse CA cert")
					}
					certs = append(certs, cert)
				}
				conf := trust.StrategyConfig{
					CACerts: &trust.CACertsConfig{
						CACerts: trust.NewCACertsSource(certs),
					},
				}
				trustStrategy, err = conf.Build()
				if err != nil {
					agentlg.With(
						zap.Error(err),
					).Fatal("error configuring CA Certs trust strategy")
				}
			case v1beta1.TrustStrategyInsecure:
				agentlg.Warn(chalk.Bold.NewStyle().WithForeground(chalk.Yellow).Style(
					"*** Using insecure trust strategy. This is not recommended. ***",
				))
				conf := trust.StrategyConfig{
					Insecure: &trust.InsecureConfig{},
				}
				trustStrategy, err = conf.Build()
				if err != nil {
					agentlg.With(
						zap.Error(err),
					).Fatal("error configuring insecure trust strategy")
				}
			}

			token, err := tokens.ParseHex(tokenData)
			if err != nil {
				agentlg.With(
					zap.Error(err),
					zap.String("token", fmt.Sprintf("[redacted (len: %d)]", len(tokenData))),
				).Error("failed to parse token")
			}
			bootstrapper = &bootstrap.ClientConfig{
				Capability:    wellknown.CapabilityMetrics,
				Token:         token,
				Endpoint:      agentConfig.Spec.GatewayAddress,
				TrustStrategy: trustStrategy,
			}
		}
	}

	p, err := agent.New(ctx, agentConfig,
		agent.WithBootstrapper(bootstrapper),
	)
	if err != nil {
		agentlg.Error(err)
		return
	}
	if err := p.ListenAndServe(ctx); err != nil {
		agentlg.Error(err)
	}
}

func runLoggingControllers(ctx context.Context) error {
	ctrl.SetLogger(ctrlzap.New(
		ctrlzap.Level(util.Must(zapcore.ParseLevel(agentLogLevel))),
		ctrlzap.Encoder(zapcore.NewConsoleEncoder(util.EncoderConfig)),
	))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     "0",
		Port:                   9443,
		HealthProbeBindAddress: ":8081",
		LeaderElection:         false,
		LeaderElectionID:       "98e737d4.opni.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	if err = (&controllers.LoggingReconciler{}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Logging")
		return err
	}

	if err = (&controllers.LogAdapterReconciler{}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LogAdapter")
		return err
	}

	if err = (&controllers.DataPrepperReconciler{}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DataPrepper")
		return err
	}

	if err = (&v1beta2.LogAdapter{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "LogAdapter")
		return err
	}

	if !enableMetrics {
		if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up health check")
			return err
		}
		if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			return err
		}
	}

	ctx, _ = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return err
	}
	return nil
}

func runEventsCollector(ctx context.Context) error {
	collector := events.NewEventCollector(ctx, eventOutputEndpoint)
	return collector.Run(ctx.Done())
}
