package certs

import (
	"context"
	"crypto/tls"
	"crypto/x509"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MockCAName = "mock-ca-cert"

	cacrtpem = `
-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUZ7LetBVDFdTEtJ61KjIpIm9tRPgwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMjEyMDUyMTQ0MDhaFw0zMjEy
MDIyMTQ0MDhaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDYXebtQJ3veLHvmMC59aJtHJlNMKoQvTKKgDFGBnga
R3/ls9ELx1ZOTiWfgLuIU5NXj8YdoSWnDojLEnryXzCnLHBMitLNW0G4VNlVAoxa
JAX7/ELM1W1WUxvTHHDlnZQW/SLzLkjbkuiZHbF2N/Wkjs8mVsFimN3Lw+6UlhPC
7r2dB3JBar9+z2ep+m3jrrfy0K2VaQrSjePQYMDY3XIqe4vXRowv+toa9qBcR8dV
pDwRRXpD9xmwfKOvS0KbT9Fn6rid7XSW01tWMtJMWmCViTu5RPaJI1oRQclPCLmu
qPpFkZZOvg4wN76Q1aKvQe6ZXf3XV8W7Wxuil75q0gAdAgMBAAGjUzBRMB0GA1Ud
DgQWBBTutlWlUw5z0YdaRiSzLzCikA95CDAfBgNVHSMEGDAWgBTutlWlUw5z0Yda
RiSzLzCikA95CDAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQA7
TKXcacp5YubyvUefJ7DjZD0Yr2FkWhjD6Ph4Hi0/2cAnJEnoPhvSTIzRQIpPSJHF
zcnubGVuaLs72RNaEilwT5W35wsisDNjXB8JCcYgoAG/wt1vQfqq5tfCpMLHOw2W
D2WGAQom4rblgmhm6aV7idaTh6v2cMinpHJiYzwgEKhf6uNAS59pGcCHExmwHWOY
yPL92Z3o3hG3p6NJPNJ3qw5ILLGl1RpwmRJTk1qXWnqRsFCugQSSM6RFeE+8ZK+y
h00nTWrTnL42g4fZMwJxXEGawKp7b+SAcHRbUsEU8BIwgn/yUsTZphHkLGv6I2nV
T3+xCPypB1tj3eCaDoeN
-----END CERTIFICATE-----
`
	cakeypem = `
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDYXebtQJ3veLHv
mMC59aJtHJlNMKoQvTKKgDFGBngaR3/ls9ELx1ZOTiWfgLuIU5NXj8YdoSWnDojL
EnryXzCnLHBMitLNW0G4VNlVAoxaJAX7/ELM1W1WUxvTHHDlnZQW/SLzLkjbkuiZ
HbF2N/Wkjs8mVsFimN3Lw+6UlhPC7r2dB3JBar9+z2ep+m3jrrfy0K2VaQrSjePQ
YMDY3XIqe4vXRowv+toa9qBcR8dVpDwRRXpD9xmwfKOvS0KbT9Fn6rid7XSW01tW
MtJMWmCViTu5RPaJI1oRQclPCLmuqPpFkZZOvg4wN76Q1aKvQe6ZXf3XV8W7Wxui
l75q0gAdAgMBAAECggEBAMR3N7pVQ1Pwf3n1dYMmFVAIePeLadF7SspSrutL8oDC
TdNRHVAZuDewZB9acG7QnOkUZyv+aMcxvmrPJA6y+uXBx1Lpd5L6+0ka2qGDh9hN
/5UZMbr3TanmG0zt9WG6XX8majbw3z1qP4TRXpPfKlE7T8QbYMxbzII7LoeDYvxL
yv7KQMJZhMnir/mLAuLuTVRnjh/FMca4Ed134c+6/LGEtU+iJXmvjgPaaXRNQUrL
njKQbTuW1PPinZfDesbsrg0uQy/lHjSrM4urAfNpIgOT8WkMc7vqQXmdHmDeqoim
fa+ubch0WOSz87B3vXobrOnQqi569xiF1x+4zjkYZiECgYEA83JF27vLwUpF11Bp
6LyUcWsdNg6p6ccB4rtuz1Qv+zNM2m+n8EyCdQsAuvFP8zS+8tr/cjkNaZXp3NjP
ObFVf6+CbY8SG3XQ06ktkld50ujq+0aTCFftORYNd42vulEAOaXyP5oGPdj0lOHA
Enkeehk3EMKTKXQ65dSoGq91bfUCgYEA44YnFX0ze/6V5vipIRiF1YRUMOwzVmN/
0Z3xi+R+eCESgOmg8/c8vNuCooQyDC6AutPyhAonyqtIEYD2iFVpJxf9QtYjwKQ3
FcNNQ8iN/FzXFXSGIBwL2S8BQ8mCEwhbd8d6j1ywM8ur7/+THjIDKS1SRHf8V8+8
fXMdhO5QiIkCgYEAnQG3Ekck2u1m672eAI8XAar+dO2yIebKTYtqpOZ753undj2K
xwzhGlFVUDvvvz/mYsRg+S7Yep9H67ocs+2t4aK08KnUGMe8PbYfgQFPvXmgixxy
GXBzu1yApPlJO1WgWo2vFdvlaJ/y5c5OzNs2j7KRdAq5VIP0tGOZY1SD3L0CgYAy
MnfPAudn9NwnsDbISXvFhsN4Y7RT2/HoUltnTMsmP82wSVssWCC7XgatSlMsYtod
3gMEZKUwzqdAzV4W6Bkh+eXzaAFNUC2jDIqwaMACrIz7e9DXprhqezdhOEUNY+ui
Oo1ssbtiQg42DgHsSIZwAELFPl+bFAb2+n3JxTZZWQKBgCWE2VTYSRltb18BzjEI
YuUtKg0ZMqj01wEwPYqUb3LnO2FiPcuwg5PXVTRI5diqNj9vrRoUKM+QtUWzhIiz
0nnJ/aVR1Am+u3CBYIZ3dcOwmsB0SWLODFiAS5yxjpgY8y29p3Ab5MOnMGxQXnLH
dGj4cKqCY/3lRzrZ+f6n545x
-----END PRIVATE KEY-----
`
)

type TestCertManager struct{}

func (m *TestCertManager) PopulateK8sObjects(ctx context.Context, client ctrlclient.Client, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockCAName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"ca.crt": cacrtpem,
			"ca.key": cakeypem,
		},
	}
	return client.Create(ctx, secret)
}

func (m *TestCertManager) GenerateRootCACert() error {
	return nil
}

func (m *TestCertManager) GenerateTransportCA() error {
	return nil
}

func (m *TestCertManager) GenerateHTTPCA() error {
	return nil
}

func (m *TestCertManager) GenerateClientCert(user string) error {
	return nil
}

func (m *TestCertManager) GetTransportRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(cacrtpem))
	return pool, nil
}

func (m *TestCertManager) GetHTTPRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(cacrtpem))
	return pool, nil
}

func (m *TestCertManager) GetClientCertificate(user string) (tls.Certificate, error) {
	return tls.Certificate{}, nil
}

func (m *TestCertManager) GetTransportCARef(_ context.Context) (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: MockCAName,
	}, nil
}

func (m *TestCertManager) GetHTTPCARef(_ context.Context) (corev1.LocalObjectReference, error) {
	return corev1.LocalObjectReference{
		Name: MockCAName,
	}, nil
}
