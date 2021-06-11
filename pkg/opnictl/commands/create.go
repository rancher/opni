// Package commands contains the opnictl sub-commands.
package commands

import (
	"context"
	"fmt"
	"time"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/filter"
	"github.com/banzaicloud/logging-operator/pkg/sdk/model/output"
	"github.com/rancher/opni/api/v1alpha1"
	. "github.com/rancher/opni/pkg/opnictl/common"
	"github.com/rancher/opni/pkg/providers"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClusterOutputName = "aiops-demo-log-output"
	ClusterFlowName   = "aiops-demo-log-flow"
)

var opniDemo = &v1alpha1.OpniDemo{}
var namespace string

var clusterOutput = &loggingv1beta1.ClusterOutput{
	ObjectMeta: metav1.ObjectMeta{
		Name: ClusterOutputName,
	},
	Spec: loggingv1beta1.ClusterOutputSpec{
		OutputSpec: loggingv1beta1.OutputSpec{
			HTTPOutput: &output.HTTPOutputConfig{
				Endpoint:    fmt.Sprintf("http://payload-receiver-service.%s.svc", namespace),
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

var clusterFlow = &loggingv1beta1.ClusterFlow{
	ObjectMeta: metav1.ObjectMeta{
		Name: ClusterFlowName,
	},
	Spec: loggingv1beta1.ClusterFlowSpec{
		Match: []loggingv1beta1.ClusterMatch{
			{
				ClusterExclude: &loggingv1beta1.ClusterExclude{
					Namespaces: []string{
						namespace,
					},
				},
			},
			{
				ClusterSelect: &loggingv1beta1.ClusterSelect{},
			},
		},
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
							Pattern: "^\n$",
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
			ClusterOutputName,
		},
	},
}

var CreateCmd = &cobra.Command{
	Use:   "create resource",
	Short: "Create new Opni resources",
	Long:  "See subcommands for more information.",
}

var CreateDemoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Create a new opni demo cluster",
	Long: fmt.Sprintf(`
This command will install opni into the selected namespace using the Demo API.
For more information about the Demo API, run %s.

Your current kubeconfig context will be used to select the cluster to operate
on, unless the --context flag is provided to select a specific context.`,
		chalk.Bold.TextStyle("opnictl help apis")),
	Run: func(cmd *cobra.Command, args []string) {
		cli := cliutil.CreateClientOrDie()

		provider := providers.Detect(cli)

		opniDemo.Namespace = namespace
		clusterOutput.Namespace = namespace
		clusterFlow.Namespace = namespace

		opniDemo.Spec.Components = v1alpha1.ComponentsSpec{
			Infra: v1alpha1.InfraStack{
				HelmController:       provider == providers.Unknown,
				LocalPathProvisioner: provider != providers.K3S,
			},
			Opni: v1alpha1.OpniStack{
				Minio:          true,
				Nats:           true,
				Elastic:        true,
				RancherLogging: opniDemo.Spec.Quickstart,
				Traefik:        opniDemo.Spec.Quickstart && provider != providers.K3S,
			},
		}

		if err := cli.Create(context.Background(), opniDemo); errors.IsAlreadyExists(err) {
			Log.Info(err.Error())
		}

		p := mpb.New()

		waitCtx, ca := context.WithTimeout(context.Background(), TimeoutFlagValue)

		waitingSpinner := p.AddSpinner(1,
			mpb.AppendDecorators(
				decor.OnComplete(decor.Name(chalk.Bold.TextStyle("Waiting for resource to become ready..."), decor.WCSyncSpaceR),
					chalk.Bold.TextStyle("Done."),
				),
			),
			mpb.BarFillerMiddleware(
				cliutil.CheckBarFiller(waitCtx, func(c context.Context) bool {
					return waitCtx == nil
				})),
			mpb.BarWidth(1),
		)
		conds := map[string]*mpb.Bar{}

		go func() {
			<-waitCtx.Done()
			waitingSpinner.Increment()
		}()
		defer ca()
		wait.PollImmediateUntil(500*time.Millisecond, func() (done bool, err error) {
			obj := &v1alpha1.OpniDemo{}
			err = cli.Get(waitCtx, client.ObjectKeyFromObject(opniDemo), obj)
			if client.IgnoreNotFound(err) != nil {
				Log.Error(err.Error())
				return false, err
			}
			state := obj.Status.State
			conditions := obj.Status.Conditions

			if state == "Ready" {
				waitingSpinner.Increment()
				done = true
			}

			for _, cond := range conditions {
				if _, ok := conds[cond]; !ok {
					conds[cond] = p.AddSpinner(1,
						mpb.AppendDecorators(
							func(cond string) decor.Decorator {
								done := false
								var doneText string
								return decor.Any(func(s decor.Statistics) string {
									if done {
										return doneText
									}
									if s.Completed || waitCtx.Err() != nil {
										done = true
										if waitCtx.Err() == nil {
											doneText = chalk.Bold.TextStyle(chalk.Green.Color("[Done] ")) + chalk.Italic.TextStyle(cond)
										} else {
											doneText = chalk.Bold.TextStyle(chalk.Red.Color("[Timed Out] ")) + chalk.Italic.TextStyle(cond)
										}
										return doneText
									} else {
										return chalk.Bold.TextStyle(chalk.Blue.Color(cond))
									}
								}, decor.WCSyncSpaceR)
							}(cond),
						),
						mpb.BarFillerMiddleware(
							cliutil.CheckBarFiller(waitCtx, func(c context.Context) bool {
								return waitCtx.Err() == nil
							}),
						),
						mpb.BarWidth(1),
					)
					go func(cond string) {
						<-waitCtx.Done()
						if !conds[cond].Completed() {
							conds[cond].Increment()
						}
					}(cond)
				}
			}

			for k, v := range conds {
				found := false
				for _, cond := range conditions {
					if k == cond {
						found = true
						break
					}
				}
				if !found {
					v.Increment()
				}
			}
			return done, nil
		}, waitCtx.Done())

		p.Wait()

		if err := cli.Create(context.Background(), clusterOutput); errors.IsAlreadyExists(err) {
			Log.Info(err.Error())
		} else if err != nil {
			Log.Error(err.Error())
		}
		if err := cli.Create(context.Background(), clusterFlow); errors.IsAlreadyExists(err) {
			Log.Info(err.Error())
		} else if err != nil {
			Log.Error(err.Error())
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateDemoCmd)
	CreateDemoCmd.Flags().StringVar(&opniDemo.Name, "name", "opni-demo", "resource name")
	CreateDemoCmd.Flags().StringVar(&namespace, "namespace", "opni-demo", "namespace to install resources to")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.MinioAccessKey, "minio-access-key", "minioadmin", "minio access key")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.MinioSecretKey, "minio-secret-key", "minioadmin", "minio access key")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.MinioVersion, "minio-version", "8.0.10", "minio chart version")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.NatsVersion, "nats-version", "2.2.1", "nats chart version")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.NatsPassword, "nats-password", "password", "nats chart version")
	CreateDemoCmd.Flags().IntVar(&opniDemo.Spec.NatsReplicas, "nats-replicas", 3, "nats pod replica count")
	CreateDemoCmd.Flags().IntVar(&opniDemo.Spec.NatsMaxPayload, "nats-max-payload", 10485760, "nats maximum payload")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.NvidiaVersion, "nvidia-version", "1.0.0-beta6", "nvidia plugin version")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.ElasticsearchUser, "elasticsearch-user", "admin", "elasticsearch username")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.ElasticsearchPassword, "elasticsearch-password", "admin", "elasticsearch password")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.TraefikVersion, "traefik-version", "v9.18.3", "traefik chart version")
	CreateDemoCmd.Flags().StringVar(&opniDemo.Spec.NulogServiceCpuRequest, "nulog-service-cpu-request", "1", "CPU resource request for nulog control-plane service")
	CreateDemoCmd.Flags().BoolVar(&opniDemo.Spec.Quickstart, "quickstart", false, "quickstart mode")
}
