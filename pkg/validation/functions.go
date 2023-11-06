package validation

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/bufbuild/protovalidate-go"
	"github.com/distribution/reference"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/prometheus/common/model"
	"golang.org/x/mod/module"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func lib(env *cel.Env) []cel.EnvOption {
	tr := env.CELTypeProvider().(*types.Registry)
	tr.RegisterMessage(&PEMBlock{})
	tr.RegisterMessage(&X509Certificate{})

	return []cel.EnvOption{
		cel.Function("isValidImageRef",
			cel.MemberOverload("str_valid_oci_image_bool",
				[]*cel.Type{cel.StringType},
				types.BoolType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					str, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					_, err := reference.ParseNormalizedNamed(str)
					if err != nil {
						return types.WrapErr(err)
					}
					return types.True
				}),
			),
		),
		cel.Function("isValidListenAddress",
			cel.MemberOverload("str_valid_listen_address_bool",
				[]*cel.Type{cel.StringType},
				types.BoolType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					str, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					host, port, err := net.SplitHostPort(str)
					if err != nil {
						return types.WrapErr(err)
					}
					if host == "" && port == "" {
						return types.NewErr("address cannot be empty")
					}
					if host != "" {
						if url, err := url.Parse(str); err == nil && url.IsAbs() {
							if url.Scheme == "unix" {
								return types.True
							}
						}
						if ip := net.ParseIP(host); ip == nil {
							if addrs, err := net.LookupHost(host); err != nil || len(addrs) == 0 {
								return types.WrapErr(errors.New("invalid IP address"))
							}
						}
					}
					if port != "" {
						if portNumber, err := strconv.Atoi(port); err != nil {
							return types.WrapErr(err)
						} else if portNumber < 0 || portNumber > 65535 {
							return types.WrapErr(errors.New("port number out of range"))
						}
					}
					return types.True
				}),
			),
		),
		cel.Function("prometheusDuration",
			cel.Overload("str_prom_model_duration",
				[]*cel.Type{cel.StringType},
				cel.DurationType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					str, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					d, err := model.ParseDuration(str)
					if err != nil {
						return types.WrapErr(err)
					}
					return types.Duration{Duration: time.Duration(d)}
				}),
			),
		),
		cel.Function("pemDecodeBlock",
			cel.MemberOverload("str_decode_pem_block",
				[]*cel.Type{cel.StringType},
				cel.ObjectType("validate.PEMBlock"),
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					str, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					block, _ := pem.Decode([]byte(str))
					if block == nil {
						return types.NewErr("malformed PEM block")
					}
					return env.CELTypeAdapter().NativeToValue(&PEMBlock{
						Type:    block.Type,
						Headers: block.Headers,
						Bytes:   block.Bytes,
					})
				}),
			),
		),
		cel.Function("pemIsValid",
			cel.MemberOverload("str_pem_is_valid",
				[]*cel.Type{cel.StringType},
				types.BoolType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					str, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					block, _ := pem.Decode([]byte(str))
					if block == nil {
						return types.False
					}
					return types.True
				}),
			),
		),
		cel.Function("x509IsValid",
			cel.MemberOverload("str_x509_is_valid",
				[]*cel.Type{cel.StringType},
				types.BoolType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					pemBlock, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					block, _ := pem.Decode([]byte(pemBlock))
					if block == nil {
						return types.False
					}
					_, err := x509.ParseCertificate(block.Bytes)
					if err != nil {
						return types.WrapErr(err)
					}
					return types.True
				}),
			),
		),
		cel.Function("x509Parse",
			cel.MemberOverload("pem_block_x509_parse",
				[]*cel.Type{cel.ObjectType("validate.PEMBlock")},
				cel.ObjectType("validate.X509Certificate"),
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					pemBlock, ok := value.Value().(*PEMBlock)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					cert, err := x509.ParseCertificate(pemBlock.Bytes)
					if err != nil {
						return types.WrapErr(err)
					}
					return env.CELTypeAdapter().NativeToValue(&X509Certificate{
						Raw:       cert.Raw,
						IsCA:      cert.IsCA,
						Issuer:    cert.Issuer.String(),
						Subject:   cert.Subject.String(),
						NotBefore: timestamppb.New(cert.NotBefore),
						NotAfter:  timestamppb.New(cert.NotAfter),
						Alg:       cert.PublicKeyAlgorithm.String(),
					})
				}),
			),
			cel.Overload("str_parse_x509",
				[]*cel.Type{cel.StringType},
				cel.ObjectType("validate.X509Certificate"),
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					str, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}
					block, _ := pem.Decode([]byte(str))
					if block == nil {
						return types.NewErr("malformed PEM block")
					}

					cert, err := x509.ParseCertificate(block.Bytes)
					if err != nil {
						return types.WrapErr(err)
					}
					return env.CELTypeAdapter().NativeToValue(&X509Certificate{
						Raw:       cert.Raw,
						IsCA:      cert.IsCA,
						Issuer:    cert.Issuer.String(),
						Subject:   cert.Subject.String(),
						NotBefore: timestamppb.New(cert.NotBefore),
						NotAfter:  timestamppb.New(cert.NotAfter),
						Alg:       cert.PublicKeyAlgorithm.String(),
					})
				}),
			),
		),
		cel.Function("checkSignatureFrom",
			cel.MemberOverload("x509_check_signature_from",
				[]*cel.Type{cel.ObjectType("validate.X509Certificate"), cel.ObjectType("validate.X509Certificate")},
				types.BoolType,
				cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
					certpb, ok := lhs.Value().(*X509Certificate)
					if !ok {
						return types.UnsupportedRefValConversionErr(lhs)
					}
					issuerpb, ok := rhs.Value().(*X509Certificate)
					if !ok {
						return types.UnsupportedRefValConversionErr(rhs)
					}

					cert, err := x509.ParseCertificate(certpb.Raw)
					if err != nil {
						return types.WrapErr(err)
					}
					issuer, err := x509.ParseCertificate(issuerpb.Raw)
					if err != nil {
						return types.WrapErr(err)
					}
					err = cert.CheckSignatureFrom(issuer)
					if err != nil {
						return types.WrapErr(err)
					}
					return types.True
				}),
			),
		),
		cel.Function("x509Verify",
			cel.Overload("x509_verify",
				[]*cel.Type{cel.ObjectType("validate.X509Certificate")},
				types.BoolType,
				cel.FunctionBinding(func(values ...ref.Val) ref.Val {
					if len(values) < 2 {
						return types.WrapErr(protovalidate.NewRuntimeErrorf("expected at least 2 arguments"))
					}
					chain := make([]*x509.Certificate, len(values))
					for i, value := range values {
						cert, ok := value.Value().(*X509Certificate)
						if !ok {
							return types.UnsupportedRefValConversionErr(value)
						}
						var err error
						chain[i], err = x509.ParseCertificate(cert.Raw)
						if err != nil {
							return types.WrapErr(err)
						}
					}

					roots := x509.NewCertPool()
					roots.AddCert(chain[0])

					intermediates := x509.NewCertPool()
					for i := 1; i < len(chain)-1; i++ {
						intermediates.AddCert(chain[i])
					}

					_, err := chain[len(chain)-1].Verify(x509.VerifyOptions{
						Roots:         roots,
						Intermediates: intermediates,
					})
					if err != nil {
						return types.WrapErr(err)
					}

					return types.True
				}),
			),
		),
		cel.Function("isValidModulePath",
			cel.MemberOverload("str_valid_module_path",
				[]*cel.Type{cel.StringType},
				types.BoolType,
				cel.UnaryBinding(func(value ref.Val) ref.Val {
					path, ok := value.Value().(string)
					if !ok {
						return types.UnsupportedRefValConversionErr(value)
					}

					if err := module.CheckPath(path); err != nil {
						return types.WrapErr(err)
					}
					return types.True
				}),
			),
		),
	}
}
