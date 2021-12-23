package commands

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/kralicky/opni-gateway/pkg/auth"
	"github.com/kralicky/opni-gateway/pkg/auth/openid"
	"github.com/kralicky/opni-gateway/pkg/config"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/gateway"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/kralicky/opni-gateway/pkg/util"
	"github.com/spf13/cobra"
)

func BuildServeCmd() *cobra.Command {
	var configLocation, listenAddr string
	var caCert, servingCert, servingKey string
	var enableMonitor bool
	var trustedProxies []string
	var managementSocket string

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

		servingCert, err := loadCerts(caCert, servingCert, servingKey)
		if err != nil {
			log.Fatalf("failed to load serving certs: %v", err)
		}

		g := gateway.NewGateway(gatewayConfig,
			gateway.WithListenAddr(listenAddr),
			gateway.WithTrustedProxies(trustedProxies),
			gateway.WithFiberMiddleware(logger.New(), compress.New()),
			gateway.WithMonitor(enableMonitor),
			gateway.WithAuthMiddleware(gatewayConfig.Spec.AuthProvider),
			gateway.WithServingCert(servingCert),
			gateway.WithManagementSocket(managementSocket),
		)

		return util.ListenReload(g)
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
	serveCmd.Flags().StringVar(&caCert, "ca-cert", "", "Path to a CA certificate")
	serveCmd.Flags().StringVar(&servingCert, "serving-cert", "", "Path to a certificate to use for serving TLS connections")
	serveCmd.Flags().StringVar(&servingKey, "serving-key", "", "Path to a key to use for serving TLS connections")
	serveCmd.Flags().StringVar(&managementSocket, "management-socket", management.DefaultManagementSocket, "Unix socket path to serve management API")
	serveCmd.MarkFlagRequired("ca-cert")
	serveCmd.MarkFlagRequired("serving-cert")
	serveCmd.MarkFlagRequired("serving-key")
	return serveCmd
}

// Returns a complete cert chain including the root CA, and a tls serving cert.
func loadCerts(cacertPath, certPath, keyPath string) (*tls.Certificate, error) {
	data, err := os.ReadFile(cacertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA cert: %v", err)
	}
	root, err := util.ParsePEMEncodedCertChain(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA cert: %v", err)
	}
	if len(root) != 1 {
		return nil, fmt.Errorf("failed to parse CA cert: expected one certificate in chain, got %d", len(root))
	}
	rootCA := root[0]
	servingCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}
	servingRootData := servingCert.Certificate[len(servingCert.Certificate)-1]
	servingRoot, err := x509.ParseCertificate(servingRootData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse serving root certificate: %w", err)
	}
	if !rootCA.Equal(servingRoot) {
		servingCert.Certificate = append(servingCert.Certificate, rootCA.Raw)
	}
	return &servingCert, nil
}
