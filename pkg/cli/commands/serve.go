package commands

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
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
	var servingCACert, servingCert, servingKey string
	var enableMonitor bool
	var trustedProxies []string

	run := func() error {
		if configLocation == "" {
			// find config file
			path, err := config.FindConfig()
			if err != nil {
				if errors.Is(err, config.ErrConfigNotFound) {
					wd, _ := os.Getwd()
					log.Fatalf(`could not find a config file in ["%s","/etc/opni-gateway"], and --config was not given`, wd)
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

		rootCAs, keypair, err := loadCerts(servingCACert, servingCert, servingKey)
		if err != nil {
			log.Fatalf("failed to load serving certs: %v", err)
		}

		g := gateway.NewGateway(gatewayConfig,
			gateway.WithListenAddr(listenAddr),
			gateway.WithTrustedProxies(trustedProxies),
			gateway.WithFiberMiddleware(logger.New(), compress.New()),
			gateway.WithMonitor(enableMonitor),
			gateway.WithAuthMiddleware(gatewayConfig.Spec.AuthProvider),
			gateway.WithRootCA(rootCAs),
			gateway.WithKeypair(keypair),
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
	serveCmd.Flags().StringVar(&servingCACert, "serving-ca-cert", "", "Path to a CA certificate to use for serving TLS connections")
	serveCmd.Flags().StringVar(&servingCert, "serving-cert", "", "Path to a certificate to use for serving TLS connections")
	serveCmd.Flags().StringVar(&servingKey, "serving-key", "", "Path to a key to use for serving TLS connections")
	return serveCmd
}

func loadCerts(cacertPath, certPath, keyPath string) (*x509.Certificate, *tls.Certificate, error) {
	var rootCA *x509.Certificate
	if cacertPath != "" {
		data, err := os.ReadFile(cacertPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load CA cert: %v", err)
		}
		der, rest := pem.Decode(data)
		if len(rest) > 0 {
			return nil, nil, fmt.Errorf("CA cert file must only contain one PEM block")
		}
		if der == nil {
			return nil, nil, fmt.Errorf("failed to decode CA cert")
		}
		rootCA, err = x509.ParseCertificate(der.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse CA cert: %v", err)
		}
	}

	keypair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	return rootCA, &keypair, nil
}
