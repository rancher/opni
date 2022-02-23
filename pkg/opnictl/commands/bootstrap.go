package commands

import (
	"context"
	"errors"
	"fmt"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/output"
	"github.com/rancher/opni/apis/v2beta1"
	registerclient "github.com/rancher/opni/pkg/multicluster/client"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type bootstrapFlagVars struct {
	apiAddress        string
	opensearchAddress string
	useRancherLogging bool
	insecure          bool
}

const (
	clusterFlowName   = "multicluster-flow"
	clusterOutputName = "multicluster-output"
	systemNamespace   = "opni-system"
)

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
	command.Flags().BoolVar(&vars.insecure, "insecure-disable-ssl-verify", false, "disable SSL verification for Opensearch endpoint")

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

	err = createDataPrepper(cmd.Context(), args[0], username, id)
	if err != nil {
		return err
	}

	if !vars.useRancherLogging {
		err = createOpniClusterOutput(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		err = createOpniClusterFlow(cmd.Context(), id)
		if err != nil {
			return err
		}
	}

	return nil
}

func createDataPrepper(ctx context.Context, name string, username string, clusterID string) error {
	dataPrepper := &v2beta1.DataPrepper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "opni-system",
		},
		Spec: v2beta1.DataPrepperSpec{
			Opensearch: &v2beta1.OpensearchSpec{
				Endpoint:                 vars.opensearchAddress,
				InsecureDisableSSLVerify: vars.insecure,
			},
			Username: username,
			PasswordFrom: &corev1.SecretKeySelector{
				Key: "password",
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "indexing-password",
				},
			},
			ClusterID: clusterID,
		},
	}
	return common.K8sClient.Create(ctx, dataPrepper)
}

func createOpniClusterOutput(ctx context.Context, name string) error {
	clusterOutput := &loggingv1beta1.ClusterOutput{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterOutputName,
			Namespace: systemNamespace,
		},
		Spec: loggingv1beta1.ClusterOutputSpec{
			OutputSpec: loggingv1beta1.OutputSpec{
				HTTPOutput: &output.HTTPOutputConfig{
					Buffer: &output.Buffer{
						FlushInterval:         "30s",
						FlushMode:             "interval",
						FlushThreadCount:      4,
						QueuedChunksLimitSize: 300,
						Type:                  "file",
					},
					Endpoint:  fmt.Sprintf("http://%s.opni-system:2021", name),
					JsonArray: true,
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
			Namespace: systemNamespace,
		},
		Spec: loggingv1beta1.ClusterFlowSpec{
			Match: []loggingv1beta1.ClusterMatch{
				{
					ClusterSelect: &loggingv1beta1.ClusterSelect{},
				},
			},
			Filters: []loggingv1beta1.Filter{
				{
					RecordTransformer: &filter.RecordTransformer{
						Records: []filter.Record{
							{
								"cluster_id": clusterID,
							},
						},
					},
				},
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
			},
			GlobalOutputRefs: []string{
				clusterOutputName,
			},
		},
	}

	return common.K8sClient.Create(ctx, clusterFlow)
}
