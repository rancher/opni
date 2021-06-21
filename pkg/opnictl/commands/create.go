// Package commands contains the opnictl sub-commands.
package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher/opni/api/v1alpha1"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/rancher/opni/pkg/providers"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildCreateDemoCmd() *cobra.Command {
	var opniDemo = &v1alpha1.OpniDemo{}
	var createDemoCmd = &cobra.Command{
		Use:   "demo",
		Short: "Create a new opni demo cluster",
		Long: fmt.Sprintf(`
		This command will install opni into the selected namespace using the Demo API.
		For more information about the Demo API, run %s.
		
		Your current kubeconfig context will be used to select the cluster to operate
		on, unless the --context flag is provided to select a specific context.`,
			chalk.Bold.TextStyle("opnictl help apis")),
		RunE: func(cmd *cobra.Command, args []string) error {
			var loggingValues = map[string]intstr.IntOrString{}
			provider, err := providers.Detect(cmd.Context(), common.K8sClient)
			if err != nil {
				return err
			}

			switch provider {
			case providers.K3S:
				loggingValues["additionalLoggingSources.k3s.enabled"] = intstr.FromString("true")
				loggingValues["systemdLogPath"] = intstr.FromString("/var/log/journal")
			case providers.RKE2:
				loggingValues["additionalLoggingSources.rke2.enabled"] = intstr.FromString("true")
			case providers.RKE:
				loggingValues["additionalLoggingSources.rke.enabled"] = intstr.FromString("true")
			}

			opniDemo.Spec.Components = v1alpha1.ComponentsSpec{
				Infra: v1alpha1.InfraStack{
					HelmController:       provider == providers.Unknown,
					LocalPathProvisioner: provider != providers.K3S,
				},
				Opni: v1alpha1.OpniStack{
					Minio: v1alpha1.ChartOptions{
						Enabled: true,
					},
					Nats: v1alpha1.ChartOptions{
						Enabled: true,
					},
					Elastic: v1alpha1.ChartOptions{
						Enabled: true,
						Set: map[string]intstr.IntOrString{
							"kibana.service.type": intstr.FromString("NodePort"),
						},
					},
					RancherLogging: v1alpha1.ChartOptions{
						Enabled: opniDemo.Spec.Quickstart,
						Set:     loggingValues,
					},
				},
			}

			// Create default namespace if it already exists and the user has not
			// requested to use a different namespace
			// Note that we are not going to make the namespace controlled by the
			// opnidemo CR
			if opniDemo.Namespace == common.DefaultOpniDemoNamespace {
				if err := common.K8sClient.Create(cmd.Context(), &corev1.Namespace{
					ObjectMeta: v1.ObjectMeta{
						Name: common.DefaultOpniDemoNamespace,
					},
				}); errors.IsAlreadyExists(err) {
					common.Log.Debug(err)
				} else if err != nil {
					return err
				}
			}

			if err := common.K8sClient.Create(cmd.Context(), opniDemo); errors.IsAlreadyExists(err) {
				common.Log.Info(err.Error())
			} else if err != nil {
				return err
			}

			p := mpb.New()

			waitCtx, ca := context.WithTimeout(cmd.Context(), common.TimeoutFlagValue)

			waitingSpinner := p.AddSpinner(1,
				mpb.AppendDecorators(
					decor.OnComplete(decor.Name(chalk.Bold.TextStyle("Waiting for resource to become ready..."), decor.WCSyncSpaceR),
						chalk.Bold.TextStyle("Done."),
					),
				),
				mpb.BarFillerMiddleware(
					cliutil.CheckBarFiller(waitCtx, func(c context.Context) bool {
						return waitCtx.Err() == nil
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
				err = common.K8sClient.Get(waitCtx, client.ObjectKeyFromObject(opniDemo), opniDemo)
				if client.IgnoreNotFound(err) != nil {
					common.Log.Error(err.Error())
					return false, err
				}
				state := opniDemo.Status.State
				conditions := opniDemo.Status.Conditions

				if state == "Ready" {
					waitingSpinner.Increment()
					done = true
					for _, v := range conds {
						v.Increment()
					}
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
										}
										return chalk.Bold.TextStyle(chalk.Blue.Color(cond))
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
			return nil
		},
	}

	createDemoCmd.Flags().StringVar(&opniDemo.Name, "name", common.DefaultOpniDemoName, "resource name")
	createDemoCmd.Flags().StringVar(&opniDemo.Namespace, "namespace", common.DefaultOpniDemoNamespace, "namespace to install resources to")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.MinioAccessKey, "minio-access-key", common.DefaultOpniDemoMinioAccessKey, "minio access key")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.MinioSecretKey, "minio-secret-key", common.DefaultOpniDemoMinioSecretKey, "minio access key")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.MinioVersion, "minio-version", common.DefaultOpniDemoMinioVersion, "minio chart version")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.NatsVersion, "nats-version", common.DefaultOpniDemoNatsVersion, "nats chart version")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.NatsPassword, "nats-password", common.DefaultOpniDemoNatsPassword, "nats chart version")
	createDemoCmd.Flags().IntVar(&opniDemo.Spec.NatsReplicas, "nats-replicas", common.DefaultOpniDemoNatsReplicas, "nats pod replica count")
	createDemoCmd.Flags().IntVar(&opniDemo.Spec.NatsMaxPayload, "nats-max-payload", common.DefaultOpniDemoNatsMaxPayload, "nats maximum payload")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.NvidiaVersion, "nvidia-version", common.DefaultOpniDemoNvidiaVersion, "nvidia plugin version")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.ElasticsearchUser, "elasticsearch-user", common.DefaultOpniDemoElasticUser, "elasticsearch username")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.ElasticsearchPassword, "elasticsearch-password", common.DefaultOpniDemoElasticPassword, "elasticsearch password")
	createDemoCmd.Flags().StringVar(&opniDemo.Spec.NulogServiceCPURequest, "nulog-service-cpu-request", common.DefaultOpniDemoNulogServiceCPURequest, "CPU resource request for nulog control-plane service")
	createDemoCmd.Flags().BoolVar(&opniDemo.Spec.Quickstart, "quickstart", common.DefaultOpniDemoQuickstart, "quickstart mode")

	return createDemoCmd
}

func BuildCreateCmd() *cobra.Command {
	var createCmd = &cobra.Command{
		Use:   "create resource",
		Short: "Create new Opni resources",
		Long:  "See subcommands for more information.",
	}
	createCmd.AddCommand(BuildCreateDemoCmd())
	return createCmd
}
