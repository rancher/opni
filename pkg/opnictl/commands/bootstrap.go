package commands

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/output"
	"github.com/banzaicloud/operator-tools/pkg/secret"
	registerclient "github.com/rancher/opni/pkg/multicluster/client"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

type bootstrapFlagVars struct {
	apiAddress        string
	opensearchAddress string
	useRancherLogging bool
}

var (
	vars = &bootstrapFlagVars{}
)

func BuildBoostrapCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "bootstrap",
		Short: "bootstrap a logging cluster",
		RunE:  bootstrapCluster,
	}

	command.Flags().StringVar(&vars.apiAddress, "api", "http://localhost:8082", "address of the multicluster API")
	command.Flags().StringVar(&vars.opensearchAddress, "opensearch", "https://localhost:9200", "address of the Opensearch endpoint")
	command.Flags().BoolVar(&vars.useRancherLogging, "rancher-logging", false, "use rancher-logging app for shipping logs")

	return command
}

func bootstrapCluster(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("bootstrap requires exactly 1 argument; the cluster name to register")
	}

	id, err := registerclient.RegisterCluster(args[0], vars.apiAddress)
	if err != nil {
		return err
	}

	username, password, err := registerclient.FetchIndexingCredentials(id, vars.apiAddress)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "indexing-password",
			Namespace: func() string {
				if vars.useRancherLogging {
					return "cattle-logging-system"
				}
				return "opni-system"
			}(),
		},
		StringData: map[string]string{
			"password": password,
		},
	}
	err = common.K8sClient.Create(cmd.Context(), secret)
	if err != nil {
		return err
	}

	if !vars.useRancherLogging {
		err = createOpniClusterOutput(cmd.Context(), username)
		if err != nil {
			return err
		}
	}

	return nil
}

func createOpniClusterOutput(ctx context.Context, username string) error {
	opensearchURL, err := url.ParseRequestURI(vars.opensearchAddress)
	if err != nil {
		return err
	}

	clusterOutput := &loggingv1beta1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multicluster-outupt",
			Namespace: "opni-system",
		},
		Spec: loggingv1beta1.ClusterOutputSpec{
			OutputSpec: loggingv1beta1.OutputSpec{
				ElasticsearchOutput: &output.ElasticsearchOutput{
					Buffer: &output.Buffer{
						FlushInterval:         "30s",
						FlushMode:             "interval",
						FlushThreadCount:      4,
						QueuedChunksLimitSize: 300,
						Type:                  "file",
					},
					FlattenHashes: true,
					Host:          opensearchURL.Host,
					Port: func() int {
						port := opensearchURL.Port()

						if port != "" {
							p, err := strconv.Atoi(port)
							if err != nil {
								return 443
							}
							return p
						}

						if opensearchURL.Scheme == "http" {
							return 80
						}
						return 443
					}(),
					Scheme:           opensearchURL.Scheme,
					IndexName:        "logs",
					LogEs400Reason:   true,
					SuppressTypeName: pointer.BoolPtr(true),
					TargetTypeKey:    "_doc",
					TypeName:         "_doc",
					User:             username,
					Password: &secret.Secret{
						ValueFrom: &secret.ValueFrom{
							SecretKeyRef: &corev1.SecretKeySelector{
								Key: "password",
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "indexing-password",
								},
							},
						},
					},
				},
			},
		},
	}
	return common.K8sClient.Create(ctx, clusterOutput)
}
