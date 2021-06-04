package commands

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/rancher/opni/api/v1alpha1"
	"github.com/rancher/opni/api/v1beta1"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var opniDemo = &v1alpha1.OpniDemo{}

var CreateCmd = &cobra.Command{
	Use: "create resource",

	Short: "Create new Opni resources",
}
var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1beta1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

var DemoCmd = &cobra.Command{
	Use:   "demo-cluster",
	Short: "Create a new opni demo cluster",
	Run: func(cmd *cobra.Command, args []string) {
		clientConfig := LoadClientConfig()

		cli, err := client.New(clientConfig, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			log.Fatal(err)
		}

		nodes := &corev1.NodeList{}
		if err := cli.List(context.Background(), nodes); err != nil {
			log.Fatal(err)
		}

		var isRKE2, isK3S bool

		for _, node := range nodes.Items {
			if strings.Contains(node.Spec.ProviderID, "k3s") {
				isK3S = true
				break
			} else if strings.Contains(node.Spec.ProviderID, "rke2") {
				isRKE2 = true
				break
			}
		}

		opniDemo.Spec.Components = v1alpha1.ComponentsSpec{
			Infra: v1alpha1.InfraStack{
				HelmController:       !isK3S && !isRKE2,
				LocalPathProvisioner: !isK3S,
			},
			Opni: v1alpha1.OpniStack{
				Minio:          true,
				Nats:           true,
				Elastic:        true,
				RancherLogging: opniDemo.Spec.Quickstart,
				Traefik:        opniDemo.Spec.Quickstart,
			},
		}

		if err := cli.Create(context.Background(), opniDemo); err != nil {
			log.Fatal(err)
		}

		p := mpb.New()

		waitingSpinner := p.AddSpinner(1, mpb.SpinnerOnLeft, mpb.PrependDecorators(
			decor.OnComplete(decor.Name("Waiting..."), "Done."),
		), mpb.BarClearOnComplete())
		conds := map[string]*mpb.Bar{}

		waitCtx := context.Background()
		wait.PollImmediateUntil(500*time.Millisecond, func() (done bool, err error) {
			obj := &v1alpha1.OpniDemo{}
			err = cli.Get(waitCtx, client.ObjectKeyFromObject(opniDemo), obj)
			if client.IgnoreNotFound(err) != nil {
				log.Println(err.Error())
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
					conds[cond] = p.AddSpinner(1, mpb.SpinnerOnLeft, mpb.PrependDecorators(
						decor.OnComplete(decor.Name(cond), "Done."),
					), mpb.BarClearOnComplete())
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
			return false, nil
		}, waitCtx.Done())

		p.Wait()
	},
}

func init() {
	CreateCmd.AddCommand(DemoCmd)
	DemoCmd.Flags().StringVar(&opniDemo.Name, "name", "opni-demo", "resource name")
	DemoCmd.Flags().StringVar(&opniDemo.Namespace, "namespace", "opni-demo", "namespace to install resources to")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.MinioAccessKey, "minio-access-key", "minioadmin", "minio access key")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.MinioSecretKey, "minio-secret-key", "minioadmin", "minio access key")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.MinioVersion, "minio-version", "8.0.10", "minio chart version")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.NatsVersion, "nats-version", "2.2.1", "nats chart version")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.NatsPassword, "nats-password", "password", "nats chart version")
	DemoCmd.Flags().IntVar(&opniDemo.Spec.NatsReplicas, "nats-replicas", 3, "nats pod replica count")
	DemoCmd.Flags().IntVar(&opniDemo.Spec.NatsMaxPayload, "nats-max-payload", 10485760, "nats maximum payload")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.NvidiaVersion, "nvidia-version", "1.0.0-beta6", "nvidia plugin version")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.ElasticsearchUser, "elasticsearch-user", "admin", "elasticsearch username")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.ElasticsearchPassword, "elasticsearch-password", "admin", "elasticsearch password")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.TraefikVersion, "traefik-version", "v9.18.3", "traefik chart version")
	DemoCmd.Flags().StringVar(&opniDemo.Spec.NulogServiceCpuRequest, "nulog-service-cpu-request", "1", "CPU resource request for nulog control-plane service")
	DemoCmd.Flags().BoolVar(&opniDemo.Spec.Quickstart, "quickstart", false, "quickstart mode")
}
