package commands

import (
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/kralicky/opni-gateway/pkg/auth"
	"github.com/kralicky/opni-gateway/pkg/auth/openid"
	"github.com/kralicky/opni-gateway/pkg/config"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/gateway"
	"github.com/spf13/cobra"
)

func BuildServeCmd() *cobra.Command {
	var configLocation, listenAddr string
	var enableMonitor bool
	var trustedProxies []string

	run := func() error {
		if configLocation == "" {
			// find config file
			path, err := config.FindConfig()
			if err != nil {
				if errors.Is(err, config.ErrConfigNotFound) {
					wd, _ := os.Getwd()
					log.Fatalf("could not find a config file in [%s; /etc/opni-gateway], and --config was given", wd)
				}
				log.Fatalf("an error occurred while searching for a config file: %v", err)
			}
			log.Println("using config file:", path)
			configLocation = path
		}

		objects, err := config.LoadObjectsFromFile(configLocation)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		var gatewayConfig *v1beta1.GatewayConfig
		objects.Visit(
			func(config *v1beta1.GatewayConfig) {
				if gatewayConfig == nil {
					gatewayConfig = config
				}
			},
			func(ap *v1beta1.AuthProvider) {
				switch ap.Spec.Type {
				case "openid":
					mw, err := openid.New(ap.Spec)
					if err != nil {
						log.Fatalf("failed to create OpenID auth provider: %v", err)
					}
					if err := auth.InstallMiddleware(ap.GetName(), mw); err != nil {
						log.Fatalf("failed to install auth provider: %v", err)
					}
				default:
					log.Printf("unsupported auth provider type: %s", ap.Spec.Type)
				}
			},
		)

		g := gateway.NewGateway(gatewayConfig,
			gateway.WithListenAddr(listenAddr),
			gateway.WithTrustedProxies(trustedProxies),
			gateway.WithFiberMiddleware(logger.New(), compress.New()),
			gateway.WithMonitor(enableMonitor),
			gateway.WithAuthMiddleware(gatewayConfig.Spec.AuthProvider),
		)

		errC := make(chan error)
		sigC := make(chan os.Signal, 1)
		go func() { errC <- g.Listen() }()
		signal.Notify(sigC, syscall.SIGHUP)
		defer func() {
			signal.Stop(sigC)
			close(sigC)
			close(errC)
		}()

		// handle SIGHUP
		select {
		case err := <-errC:
			return err
		case <-sigC:
			log.Println("SIGHUP received, reloading config")
			if err := g.Shutdown(); err != nil {
				log.Fatalf("failed to shut down gateway: %v", err)
			}
			select {
			case <-time.After(5 * time.Second):
				log.Fatalf("failed to shut down gateway: timeout")
			case <-errC:
			}
			return nil
		}
	}

	serveCmd := &cobra.Command{
		Use:   "serve [flags]",
		Short: "Run the opni gateway HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			for {
				if err := run(); err != nil {
					return err
				}
			}
		},
	}

	serveCmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	serveCmd.Flags().StringVar(&listenAddr, "listen", "0.0.0.0:8080", "address:port to listen on")
	serveCmd.Flags().StringSliceVar(&trustedProxies, "trusted-proxies", []string{}, "List of trusted proxy IP addresses")
	serveCmd.Flags().BoolVar(&enableMonitor, "enable-monitor", false, "Enable the /monitor endpoint")
	return serveCmd
}
