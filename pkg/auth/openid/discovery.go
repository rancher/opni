package openid

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type DiscoverySpec struct {
	// Relative path at which to find the openid configuration. If not set,
	// defaults to '/.well-known/openid-configuration'.
	//+kubebuilder:default=/.well-known/openid-configuration
	Path *string `json:"path,omitempty"`

	// The OP's Issuer identifier. This must exactly match the issuer URL
	// obtained from the discovery endpoint, and will match the `iss' claim
	// in the ID Tokens issued by the OP.
	Issuer string `json:"issuer"`

	// Optional path to the issuer's CA Certificate.
	CACert *string `json:"cacert,omitempty"`
}

var (
	ErrIssuerMismatch         = errors.New("issuer mismatch")
	ErrMissingDiscoveryConfig = errors.New("at least one of 'discovery' or 'wellKnownConfiguration' fields must be set")
)

func isDiscoveryErrFatal(err error) bool {
	return errors.Is(err, ErrIssuerMismatch) ||
		errors.Is(err, ErrMissingDiscoveryConfig)
}

func (oc *OpenidConfig) GetWellKnownConfiguration() (*WellKnownConfiguration, error) {
	if oc.Discovery == nil && oc.WellKnownConfiguration == nil {
		return nil, ErrMissingDiscoveryConfig
	}
	if oc.WellKnownConfiguration != nil {
		return oc.WellKnownConfiguration, nil
	}

	wkc, err := fetchWellKnownConfig(oc.Discovery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch configuration from discovery endpoint: %w", err)
	}
	oc.WellKnownConfiguration = wkc
	return wkc, nil
}

func fetchWellKnownConfig(dc *DiscoverySpec) (*WellKnownConfiguration, error) {
	client := http.Client{
		// Do not redirect when fetching the openid configuration.
		// The issuer URL must be the exact URL at which the discovery
		// endpoint is located. The only valid status code is 200.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if dc.CACert != nil {
		pool := x509.NewCertPool()
		cacert, err := os.ReadFile(*dc.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		pool.AppendCertsFromPEM(cacert)
		client.Transport = otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		})
	}
	u, err := url.Parse(dc.Issuer)
	if err != nil {
		return nil, err
	}
	rel := "/.well-known/openid-configuration"
	if dc.Path != nil {
		rel = *dc.Path
	}
	u.Path = path.Join(u.Path, rel)

	ctx, ca := context.WithTimeout(context.Background(), 2*time.Second)
	defer ca()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	defer resp.Body.Close()
	var cfg WellKnownConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}
	if cfg.Issuer != dc.Issuer {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrIssuerMismatch, dc.Issuer, cfg.Issuer)
	}
	if err := cfg.CheckRequiredFields(); err != nil {
		return nil, fmt.Errorf("%w (you may need to manually set the well-known configuration)", err)
	}
	return &cfg, nil
}
