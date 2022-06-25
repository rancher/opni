package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/util"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

func BuildHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "hooks",
		Short:  "Helm hooks",
		Hidden: true,
	}

	cmd.AddCommand(BuildWaitForResourceCmd())
	return cmd
}

func BuildWaitForResourceCmd() *cobra.Command {
	var group, version, resource, namespace string
	cmd := &cobra.Command{
		Use:   "wait-for-resource [--namespace=<namespace>] [--group=<group>] [--version=<version>] --resource=<resource> <name>",
		Short: "Wait for a resource to be created",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := util.ClientOptions{
				Scheme: apis.NewScheme(),
			}
			if kc, ok := os.LookupEnv("KUBECONFIG"); ok {
				opts.Kubeconfig = &kc
			}
			restConfig, err := util.NewRestConfig(opts)
			if err != nil {
				return err
			}
			client := dynamic.NewForConfigOrDie(restConfig)

			gvr := schema.GroupVersionResource{
				Group:    group,
				Version:  version,
				Resource: resource,
			}

			fmt.Printf("Waiting for resource to be created: %s %s\n", gvr.String(), args[0])
			err = wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
				_, err := client.Resource(gvr).Namespace(namespace).Get(cmd.Context(), args[0], v1.GetOptions{})
				if err != nil {
					fmt.Println(err)
					return false, nil
				}
				return true, nil
			}, cmd.Context().Done())
			if err != nil {
				return err
			}
			fmt.Println("Resource created.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&group, "group", "g", "", "Group")
	cmd.Flags().StringVarP(&version, "version", "v", "", "Version")
	cmd.Flags().StringVarP(&resource, "resource", "r", "", "Resource")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to use")
	cmd.MarkFlagRequired("resource")
	return cmd
}
