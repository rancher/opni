package openid

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/pkg/util"
)

type WellKnownConfiguration struct {
	Issuer                            string   `json:"issuer,omitempty"`
	AuthEndpoint                      string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint                     string   `json:"token_endpoint,omitempty"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint,omitempty"`
	RevocationEndpoint                string   `json:"revocation_endpoint,omitempty"`
	JwksUri                           string   `json:"jwks_uri,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported,omitempty"`
	ResponseModesSupported            []string `json:"response_modes_supported,omitempty"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported,omitempty"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	ClaimsSupported                   []string `json:"claims_supported,omitempty"`
	RequestURIParameterSupported      bool     `json:"request_uri_parameter_supported,omitempty"`
}

type OpenidConfig struct {
	// Discovery and WellKnownConfiguration are mutually exclusive.
	// If the OP (openid provider) has a discovery endpoint, it should be
	// configured in the Discovery field, otherwise the well-known configuration
	// fields can be set manually.
	Discovery              *DiscoverySpec          `json:"discovery,omitempty"`
	WellKnownConfiguration *WellKnownConfiguration `json:"wellKnownConfiguration,omitempty"`

	// IdentifyingClaim is the claim that will be used to identify the user
	// (e.g. "sub", "email", etc). Defaults to "sub".
	//+kubebuilder:default=sub
	IdentifyingClaim string `json:"identifyingClaim,omitempty"`
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
