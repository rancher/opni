package configv1

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	errors "errors"
	"fmt"
	"os"

	cli "github.com/rancher/opni/internal/codegen/cli"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/dryrun"
	"github.com/rancher/opni/pkg/plugins/driverutil/rollback"
	"github.com/rancher/opni/pkg/tui"
	"golang.org/x/term"
)

// Enables the history terminal UI
func (h *HistoryResponse) RenderText(out cli.Writer) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		out.Println(driverutil.MarshalConfigJson(h))
		return
	}
	ui := tui.NewHistoryUI(h.GetEntries())
	ui.Run()
}

func init() {
	addExtraGatewayConfigCmd(rollback.BuildCmd("rollback", GatewayConfigContextInjector))
	addExtraGatewayConfigCmd(dryrun.BuildCmd("dry-run", GatewayConfigContextInjector,
		BuildGatewayConfigSetConfigurationCmd(),
		BuildGatewayConfigSetDefaultConfigurationCmd(),
		BuildGatewayConfigResetConfigurationCmd(),
		BuildGatewayConfigResetDefaultConfigurationCmd(),
	))
}

var ErrInsecure = errors.New("insecure")

func (m *MTLSSpec) AsTlsConfig() (*tls.Config, error) {
	var serverCaData, clientCaData, clientCertData, clientKeyData []byte
	var err error
	switch {
	case m.ClientCA != nil:
		clientCaData, err = os.ReadFile(m.GetClientCA())
		if err != nil {
			return nil, fmt.Errorf("failed to read client CA: %w", err)
		}
	case m.ClientCAData != nil:
		clientCaData = []byte(m.GetClientCAData())
	}

	switch {
	case m.ServerCA != nil:
		serverCaData, err = os.ReadFile(m.GetServerCA())
		if err != nil {
			return nil, fmt.Errorf("failed to read server CA: %w", err)
		}
	case m.ServerCAData != nil:
		serverCaData = []byte(m.GetServerCAData())
	default:
		return nil, fmt.Errorf("%w: no server CA configured", ErrInsecure)
	}

	switch {
	case m.ClientCert != nil:
		clientCertData, err = os.ReadFile(m.GetClientCert())
		if err != nil {
			return nil, fmt.Errorf("failed to read client cert: %w", err)
		}
	case m.ClientCertData != nil:
		clientCertData = []byte(m.GetClientCertData())
	default:
		return nil, fmt.Errorf("%w: no client cert configured", ErrInsecure)
	}

	switch {
	case m.ClientKey != nil:
		clientKeyData, err = os.ReadFile(m.GetClientKey())
		if err != nil {
			return nil, fmt.Errorf("failed to read client key: %w", err)
		}
	case m.ClientKeyData != nil:
		clientKeyData = []byte(m.GetClientKeyData())
	default:
		return nil, fmt.Errorf("%w: no client key configured", ErrInsecure)
	}

	clientCert, err := tls.X509KeyPair(clientCertData, clientKeyData)
	if err != nil {
		return nil, err
	}

	var clientCAPool *x509.CertPool
	if len(clientCaData) > 0 {
		clientCAPool = x509.NewCertPool()
		clientCAPool.AppendCertsFromPEM(clientCaData)
	}

	serverCAPool := x509.NewCertPool()
	serverCAPool.AppendCertsFromPEM(serverCaData)

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}, nil
}

func (c *CertsSpec) AsTlsConfig(clientAuth tls.ClientAuthType) (*tls.Config, error) {
	var caCertData, servingCertData, servingKeyData []byte
	switch {
	case c.CaCert != nil:
		data, err := os.ReadFile(c.GetCaCert())
		if err != nil {
			return nil, fmt.Errorf("failed to load CA cert: %w", err)
		}
		caCertData = data
	case c.CaCertData != nil:
		caCertData = []byte(c.GetCaCertData())
	default:
		return nil, fmt.Errorf("%w: no CA cert configured", ErrInsecure)
	}
	switch {
	case c.ServingCert != nil:
		data, err := os.ReadFile(c.GetServingCert())
		if err != nil {
			return nil, fmt.Errorf("failed to load serving cert: %w", err)
		}
		servingCertData = data
	case c.ServingCertData != nil:
		servingCertData = []byte(c.GetServingCertData())
	default:
		return nil, fmt.Errorf("%w: no serving cert configured", ErrInsecure)
	}
	switch {
	case c.ServingKey != nil:
		data, err := os.ReadFile(c.GetServingKey())
		if err != nil {
			return nil, fmt.Errorf("failed to load serving key: %w", err)
		}
		servingKeyData = data
	case c.ServingKeyData != nil:
		servingKeyData = []byte(c.GetServingKeyData())
	default:
		return nil, fmt.Errorf("%w: no serving key configured", ErrInsecure)
	}

	var block *pem.Block
	block, _ = pem.Decode(caCertData)
	if block == nil {
		return nil, errors.New("failed to decode CA cert PEM data")
	}
	rootCA, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}
	servingCert, err := tls.X509KeyPair(servingCertData, servingKeyData)
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
	caPool := x509.NewCertPool()
	caPool.AddCert(rootCA)

	if clientAuth == tls.NoClientCert {
		return &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{servingCert},
			RootCAs:      caPool,
		}, nil
	} else {
		return &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{servingCert},
			RootCAs:      caPool,
			ClientCAs:    caPool,
			ClientAuth:   clientAuth,
		}, nil
	}
}
