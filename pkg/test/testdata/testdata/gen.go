//go:generate -command cert step certificate create -f --kty=OKP --crv=Ed25519 --insecure --no-password --not-after=87600h
//go:generate cert "Example Root CA" root_ca.crt root_ca.key --profile=root-ca
//go:generate cert "Example Intermediate CA 1" intermediate_ca_1.crt intermediate_ca_1.key --profile=intermediate-ca --ca=root_ca.crt --ca-key=root_ca.key
//go:generate cert "Example Intermediate CA 2" intermediate_ca_2.crt intermediate_ca_2.key --profile=intermediate-ca --ca=intermediate_ca_1.crt --ca-key=intermediate_ca_1.key
//go:generate cert "Example Intermediate CA 3" intermediate_ca_3.crt intermediate_ca_3.key --profile=intermediate-ca --ca=intermediate_ca_2.crt --ca-key=intermediate_ca_2.key
//go:generate cert "example.com" example.com.crt example.com.key --profile=leaf --ca=intermediate_ca_3.crt --ca-key=intermediate_ca_3.key
//go:generate cert "leaf" localhost.crt localhost.key --profile=leaf --ca=root_ca.crt --ca-key=root_ca.key --san=localhost --san=
//go:generate cert "self-signed-leaf" self_signed_leaf.crt self_signed_leaf.key --profile=self-signed --subtle
//go:generate cert "Test Cortex CA" cortex/root.crt cortex/root.key --profile=intermediate-ca --ca=root_ca.crt --ca-key=root_ca.key
//go:generate cert "Test Cortex Client" cortex/client.crt cortex/client.key --profile=leaf --ca=cortex/root.crt --ca-key=cortex/root.key --san=localhost --san=127.0.0.1
//go:generate cert "Test Cortex Server" cortex/server.crt cortex/server.key --profile=leaf --ca=cortex/root.crt --ca-key=cortex/root.key --san=localhost --san=127.0.0.1

package testdata
