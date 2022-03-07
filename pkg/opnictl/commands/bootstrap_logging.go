package commands

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/rancher/opni-monitoring/pkg/b2bmac"
	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/capabilities/wellknown"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/tokens"
	loggingplugin "github.com/rancher/opni-monitoring/plugins/logging/pkg/logging"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	secretName        = "opni-opensearch-auth"
	secretKey         = "password"
	dataPrepperName   = "opni-shipper"
	clusterOutputName = "opni-output"
	clusterFlowName   = "opni-flow"
)

var (
	skipTLSVerify   bool
	rancherLogging  bool
	gatewayEndpoint string
	bootstrapToken  string

	httpClient *http.Client
)

func BuildBootstrapLoggingCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "logging cluster-name",
		Short: "bootstrap a downstream logging cluster",
		Args:  cobra.ExactArgs(1),
		RunE:  doBootstrap,
	}

	command.Flags().BoolVar(&skipTLSVerify, "insecure-skip-tls-verify", false, "skip endpoint tls verification")
	command.Flags().BoolVar(&rancherLogging, "use-rancher-logging", false, "manually configure log shipping with rancher-logging")
	command.Flags().StringVar(&gatewayEndpoint, "gateway-url", "https://localhost:8443", "upstream Opni gateway")
	command.Flags().StringVar(&bootstrapToken, "token", "", "bootstrap token")

	command.MarkFlagRequired("token")

	return command
}

func doBootstrap(cmd *cobra.Command, args []string) error {
	clusterID, err := getClusterId(cmd.Context())
	if err != nil {
		return err
	}

	sharedKeys, err := doBootstrapAuth(clusterID)
	if err != nil {
		return err
	}

	// Generate auth.  Body is empty as we are sending a get request
	nonce, sig, err := b2bmac.New512([]byte(clusterID), []byte{}, sharedKeys.ClientKey)
	if err != nil {
		return err
	}

	authHeader, err := b2bmac.EncodeAuthHeader([]byte(clusterID), nonce, sig)

	// error already checked in auth
	url, _ := getClusterDetailsURL()
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	req.Header.Add("Authorization", authHeader)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var detailsResp loggingplugin.OpensearchDetailsResponse

	if err := json.NewDecoder(resp.Body).Decode(&detailsResp); err != nil {
		return err
	}

	if err := createAuthSecret(cmd.Context(), detailsResp.Password); err != nil {
		return err
	}

	if err := createDataPrepper(
		cmd.Context(),
		detailsResp.Username,
		clusterID,
		detailsResp.ExternalURL,
	); err != nil {
		return err
	}

	if !rancherLogging {
		if err := createOpniClusterOutput(cmd.Context()); err != nil {
			return err
		}
		if err := createOpniClusterFlow(cmd.Context(), clusterID); err != nil {
			return err
		}
	}

	return nil
}

func bootstrapJoinURL() (*url.URL, error) {
	u, err := url.Parse(gatewayEndpoint)
	if err != nil {
		return nil, err
	}
	u.Scheme = "https"
	u.Path = "bootstrap/join"
	return u, nil
}

func bootstrapAuthURL() (*url.URL, error) {
	u, err := url.Parse(gatewayEndpoint)
	if err != nil {
		return nil, err
	}
	u.Scheme = "https"
	u.Path = "bootstrap/auth"
	return u, nil
}

func getClusterDetailsURL() (*url.URL, error) {
	u, err := url.Parse(gatewayEndpoint)
	if err != nil {
		return nil, err
	}
	u.Scheme = "https"
	u.Path = "logging/v1/cluster"
	return u, nil
}

func bootStrapJoin() (*bootstrap.BootstrapJoinResponse, *x509.Certificate, error) {
	url, err := bootstrapJoinURL()
	if err != nil {
		return nil, nil, err
	}

	resp, err := httpClient.Post(url.String(), "application/json", nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf(resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	bootstrapResponse := &bootstrap.BootstrapJoinResponse{}
	if err := json.Unmarshal(body, bootstrapResponse); err != nil {
		return nil, nil, err
	}

	return bootstrapResponse, resp.TLS.PeerCertificates[0], nil
}

func findValidSignature(
	token *tokens.Token,
	signatures map[string][]byte,
	pubKey interface{},
) ([]byte, error) {
	if sig, ok := signatures[token.HexID()]; ok {
		return token.VerifyDetached(sig, pubKey)
	}
	return nil, bootstrap.ErrNoValidSignature
}

func getClusterId(ctx context.Context) (string, error) {
	systemNamespace := &corev1.Namespace{}
	if err := common.K8sClient.Get(ctx, types.NamespacedName{
		Name: "kube-system",
	}, systemNamespace); err != nil {
		return "", err
	}

	return string(systemNamespace.GetUID()), nil
}

func createAuthSecret(ctx context.Context, password string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: common.NamespaceFlagValue,
		},
		StringData: map[string]string{
			secretKey: password,
		},
	}

	return common.K8sClient.Create(ctx, secret)
}

func createDataPrepper(
	ctx context.Context,
	username string,
	clusterID string,
	opensearchEndpoint string,
) error {
	dataPrepper := v2beta1.DataPrepper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataPrepperName,
			Namespace: common.NamespaceFlagValue,
		},
		Spec: v2beta1.DataPrepperSpec{
			Username: username,
			PasswordFrom: &corev1.SecretKeySelector{
				Key: secretKey,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
			Opensearch: &v2beta1.OpensearchSpec{
				Endpoint:                 opensearchEndpoint,
				InsecureDisableSSLVerify: skipTLSVerify,
			},
			ClusterID: clusterID,
		},
	}

	return common.K8sClient.Create(ctx, &dataPrepper)
}

func createOpniClusterOutput(ctx context.Context) error {
	clusterOutput := &loggingv1beta1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterOutputName,
			Namespace: common.NamespaceFlagValue,
		},
		Spec: loggingv1beta1.ClusterOutputSpec{
			OutputSpec: loggingv1beta1.OutputSpec{
				HTTPOutput: &output.HTTPOutputConfig{
					Endpoint:    fmt.Sprintf("http://%s.%s", dataPrepperName, common.NamespaceFlagValue),
					ContentType: "application/json",
					JsonArray:   true,
					Buffer: &output.Buffer{
						Tags:           "[]",
						FlushInterval:  "2s",
						ChunkLimitSize: "1mb",
					},
				},
			},
		},
	}
	return common.K8sClient.Create(ctx, clusterOutput)
}

func createOpniClusterFlow(ctx context.Context, clusterID string) error {
	clusterFlow := &loggingv1beta1.ClusterFlow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterFlowName,
			Namespace: common.NamespaceFlagValue,
		},
		Spec: loggingv1beta1.ClusterFlowSpec{
			Filters: []loggingv1beta1.Filter{
				{
					Dedot: &filter.DedotFilterConfig{
						Separator: "-",
						Nested:    true,
					},
				},
				{
					Grep: &filter.GrepConfig{
						Exclude: []filter.ExcludeSection{
							{
								Key:     "log",
								Pattern: `^\n$`,
							},
						},
					},
				},
				{
					DetectExceptions: &filter.DetectExceptions{
						Languages: []string{
							"java",
							"python",
							"go",
							"ruby",
							"js",
							"csharp",
							"php",
						},
						MultilineFlushInterval: "0.1",
					},
				},
				{
					RecordTransformer: &filter.RecordTransformer{
						Records: []filter.Record{
							{
								"cluster_id": clusterID,
							},
						},
					},
				},
			},
			Match: []loggingv1beta1.ClusterMatch{
				{
					ClusterExclude: &loggingv1beta1.ClusterExclude{
						Namespaces: []string{
							"opni-system",
						},
					},
				},
				{
					ClusterSelect: &loggingv1beta1.ClusterSelect{},
				},
			},
			GlobalOutputRefs: []string{
				clusterOutputName,
			},
		},
	}

	return common.K8sClient.Create(ctx, clusterFlow)
}

func doBootstrapAuth(clusterID string) (*keyring.SharedKeys, error) {
	// parse the token
	token, err := tokens.ParseHex(bootstrapToken)
	if err != nil {
		return nil, err
	}

	// configure http client
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skipTLSVerify,
	}

	netTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig:     tlsConfig,
	}

	httpClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: netTransport,
	}

	bootstrapResponse, serverLeafCert, err := bootStrapJoin()
	if err != nil {
		return nil, err
	}

	// find valid signature from server response
	jws, err := findValidSignature(
		token,
		bootstrapResponse.Signatures,
		serverLeafCert.PublicKey,
	)
	if err != nil {
		return nil, err
	}

	ekp := ecdh.NewEphemeralKeyPair()

	authReq, err := json.Marshal(bootstrap.BootstrapAuthRequest{
		ClientID:     clusterID,
		ClientPubKey: ekp.PublicKey,
		Capability:   wellknown.CapabilityLogs,
	})
	if err != nil {
		return nil, err
	}

	// error already checked in join
	url, _ := bootstrapAuthURL()

	req, err := http.NewRequest(http.MethodPost, url.String(), bytes.NewReader(authReq))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Request", "application/json")
	req.Header.Add("Authorization", "Bearer "+string(jws))
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: %s", bootstrap.ErrBootstrapFailed, resp.Status)
	}

	var authResp bootstrap.BootstrapAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, err
	}

	sharedSecret, err := ecdh.DeriveSharedSecret(ekp, ecdh.PeerPublicKey{
		PublicKey: authResp.ServerPubKey,
		PeerType:  ecdh.PeerTypeServer,
	})
	if err != nil {
		return nil, err
	}
	return keyring.NewSharedKeys(sharedSecret), nil
}
