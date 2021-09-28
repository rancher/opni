package commands

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/rancher/opni/apis/demo/v1alpha1"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/rancher/opni/pkg/providers"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func BuildCreateDemoCmd() *cobra.Command {
	var opniDemo = &v1alpha1.OpniDemo{}
	var deployHelmController string
	var deployNvidiaPlugin string
	var deployRancherLogging string
	var deployGpuServices string
	var loggingCrdNamespace string

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
			questions := []*survey.Question{}
			if deployHelmController == "prompt" {
				questions = append(questions, &survey.Question{
					Name: "DeployHelmController",
					Prompt: &survey.Confirm{
						Message: "Deploy the Helm Controller alongside Opni?",
						Default: false,
					},
				})
			}
			if deployRancherLogging == "prompt" {
				questions = append(questions, &survey.Question{
					Name: "DeployRancherLogging",
					Prompt: &survey.Confirm{
						Message: "Deploy the Rancher Logging Helm chart alongside Opni?",
						Default: true,
					},
				})
			}
			if deployGpuServices == "prompt" {
				questions = append(questions, &survey.Question{
					Name: "DeployGpuServices",
					Prompt: &survey.Confirm{
						Message: "Enable GPU-accelerated anomaly detection? (Requires a GPU)",
						Default: false,
					},
				})
			}

			responses := struct {
				DeployHelmController bool
				DeployRancherLogging bool
				DeployGpuServices    bool
			}{}

			if err := survey.Ask(questions, &responses); err != nil {
				return err
			}

			if deployHelmController == "prompt" {
				opniDemo.Spec.Components.Infra.DeployHelmController = responses.DeployHelmController
			} else {
				opniDemo.Spec.Components.Infra.DeployHelmController = (deployHelmController == "true")
			}

			if deployRancherLogging == "prompt" {
				opniDemo.Spec.Components.Opni.RancherLogging.Enabled = responses.DeployRancherLogging
			} else {
				opniDemo.Spec.Components.Opni.RancherLogging.Enabled = (deployRancherLogging == "true")
			}

			if deployGpuServices == "prompt" {
				opniDemo.Spec.Components.Opni.DeployGpuServices = responses.DeployGpuServices
			} else {
				opniDemo.Spec.Components.Opni.DeployGpuServices = (deployGpuServices == "true")
			}

			if opniDemo.Spec.Components.Opni.DeployGpuServices && deployNvidiaPlugin == "prompt" {
				var response bool
				if err := survey.AskOne(&survey.Confirm{
					Message: "Deploy the Nvidia plugin DaemonSet?",
					Default: true,
				}, &response); err != nil {
					return err
				}
				opniDemo.Spec.Components.Infra.DeployNvidiaPlugin = response
			} else {
				opniDemo.Spec.Components.Infra.DeployNvidiaPlugin = (deployNvidiaPlugin == "true")
			}

			if !opniDemo.Spec.Components.Opni.RancherLogging.Enabled {
				if loggingCrdNamespace == "" {
					if err := survey.AskOne(&survey.Input{
						Message: "Enter the namespace where Rancher Logging is installed:",
						Default: "cattle-logging-system",
						Help:    "This is the \"control namespace\" where the BanzaiCloud Logging Operator looks for ClusterFlow and ClusterOutput resources.",
					}, &loggingCrdNamespace); err != nil {
						return err
					}
				}
				opniDemo.Spec.LoggingCRDNamespace = &loggingCrdNamespace
			}

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
			opniDemo.Spec.Components.Opni.RancherLogging.Set = loggingValues

			opniDemo.Spec.Components.Opni.Minio = v1alpha1.ChartOptions{
				Enabled: true,
			}
			opniDemo.Spec.Components.Opni.Nats = v1alpha1.ChartOptions{
				Enabled: true,
			}
			opniDemo.Spec.Components.Opni.Elastic = v1alpha1.ChartOptions{
				Enabled: true,
				Set: map[string]intstr.IntOrString{
					"kibana.service.type": intstr.FromString("NodePort"),
				},
			}

			if opniDemo.Namespace == common.DefaultOpniDemoNamespace {
				common.CreateDefaultDemoNamespace(cmd.Context())
			}

			if err := common.K8sClient.Create(cmd.Context(), opniDemo); errors.IsAlreadyExists(err) {
				common.Log.Info(err.Error())
			} else if meta.IsNoMatchError(err) {
				common.Log.Fatal("Opni is not installed. Try running 'opnictl install' first.")
			} else if err != nil {
				fmt.Println(errors.ReasonForError(err))
				return err
			}

			return cliutil.WaitAndDisplayStatus(cmd.Context(), common.TimeoutFlagValue, common.K8sClient, opniDemo)
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
	createDemoCmd.Flags().StringVar(&loggingCrdNamespace, "logging-namespace", "", "Logging Operator Control Namespace")

	// the flags below have the following usage:
	// [unset] 			-> "prompt"
	// --flag 			-> "true"
	// --flag=true 	-> "true"
	// --flag=false -> "false"

	createDemoCmd.Flags().StringVar(&deployHelmController, "deploy-helm-controller", "prompt", "deploy the Helm Controller alongside Opni (true/false)")
	createDemoCmd.Flags().StringVar(&deployNvidiaPlugin, "deploy-nvidia-plugin", "prompt", "deploy the Nvidia GPU plugin alongside Opni (true/false)")
	createDemoCmd.Flags().StringVar(&deployRancherLogging, "deploy-rancher-logging", "prompt", "deploy the Rancher Logging Helm chart alongside Opni (true/false)")
	createDemoCmd.Flags().StringVar(&deployGpuServices, "deploy-gpu-services", "prompt", "deploy GPU-enabled Opni services (true/false)")

	createDemoCmd.Flags().Lookup("deploy-helm-controller").NoOptDefVal = "true"
	createDemoCmd.Flags().Lookup("deploy-rancher-logging").NoOptDefVal = "true"
	createDemoCmd.Flags().Lookup("deploy-nvidia-plugin").NoOptDefVal = "true"
	createDemoCmd.Flags().Lookup("deploy-gpu-services").NoOptDefVal = "true"

	return createDemoCmd
}
