package openid

import (
	"errors"
	"fmt"

	"github.com/rancher/opni-monitoring/pkg/util"
)

type WellKnownConfiguration struct {
	Issuer                            string   `json:"issuer"`
	AuthEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	RevocationEndpoint                string   `json:"revocation_endpoint"`
	JwksUri                           string   `json:"jwks_uri"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ResponseModesSupported            []string `json:"response_modes_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	RequestURIParameterSupported      bool     `json:"request_uri_parameter_supported"`
}

type OpenidConfig struct {
	// Discovery and WellKnownConfiguration are mutually exclusive.
	// If the OP (openid provider) has a discovery endpoint, it should be
	// configured in the Discovery field, otherwise the well-known configuration
	// fields can be set manually.
	Discovery              *DiscoverySpec          `json:"discovery"`
	WellKnownConfiguration *WellKnownConfiguration `json:"wellKnownConfiguration"`

	// IdentifyingClaim is the claim that will be used to identify the user
	// (e.g. "sub", "email", etc). Defaults to "sub".
	IdentifyingClaim string `json:"identifyingClaim"`
}

var ErrMissingRequiredField = errors.New("openid configuration missing required field")

func (w WellKnownConfiguration) CheckRequiredFields() error {
	if w.Issuer == "" {
		return fmt.Errorf("%w: issuer", ErrMissingRequiredField)
	}
	if w.AuthEndpoint == "" {
		return fmt.Errorf("%w: authorization_endpoint", ErrMissingRequiredField)
	}
	if w.TokenEndpoint == "" {
		return fmt.Errorf("%w: token_endpoint", ErrMissingRequiredField)
	}
	if w.UserinfoEndpoint == "" {
		return fmt.Errorf("%w: userinfo_endpoint", ErrMissingRequiredField)
	}
	if w.JwksUri == "" {
		return fmt.Errorf("%w: jwks_uri", ErrMissingRequiredField)
	}
	return nil
}

func (in *OpenidConfig) DeepCopyInto(out *OpenidConfig) {
	util.DeepCopyInto(out, in)
}

func (in *OpenidConfig) DeepCopy() *OpenidConfig {
	return util.DeepCopy(in)
}
