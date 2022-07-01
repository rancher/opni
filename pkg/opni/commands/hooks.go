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
	"k8s.io/client-go/util/jsonpath"
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
	var jsonPaths []string
	var jsonPathParsers []*jsonpath.JSONPath
	cmd := &cobra.Command{
		Use:   "wait-for-resource [--namespace=<ns>] [--group=<group>] [--version=<version>] [--jsonpath={.expr} ...] --resource=<resource> <name>",
		Short: "Wait for a resource to be created, and optionally match a JSONPath expression.",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			for i, jsonPath := range jsonPaths {
				path := jsonpath.New(fmt.Sprint(i))
				if err := path.Parse(jsonPath); err != nil {
					return err
				}
				jsonPathParsers = append(jsonPathParsers, path)
			}
			return nil
		},
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

			for i, parser := range jsonPathParsers {
				fmt.Printf("Evaluating JSONPath expression: %s\n", jsonPaths[i])
				err = wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
					obj, err := client.Resource(gvr).Namespace(namespace).Get(cmd.Context(), args[0], v1.GetOptions{})
					if err != nil {
						fmt.Println(err)
						return false, nil
					}
					parseResults, err := parser.FindResults(obj.UnstructuredContent())
					if err != nil {
						fmt.Println(err)
						return false, nil
					}
					if len(parseResults) != 1 {
						fmt.Printf("Expected 1 result, got %d\n", len(parseResults))
						return false, nil
					}
					value := parseResults[0][0]
					if value.IsNil() || value.IsZero() {
						fmt.Println("Expression did not match.")
						return false, nil
					}
					fmt.Printf("Expression matched: %v\n", value.Interface())
					return true, nil
				}, cmd.Context().Done())

				if err != nil {
					return err
				}
			}

			fmt.Println("Success.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&group, "group", "g", "", "Group")
	cmd.Flags().StringVarP(&version, "version", "v", "", "Version")
	cmd.Flags().StringVarP(&resource, "resource", "r", "", "Resource")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to use")
	cmd.Flags().StringSliceVarP(&jsonPaths, "jsonpath", "j", []string{}, "Wait for a JSONPath expression to evaluate to a non-zero value once the resource is created")
	cmd.MarkFlagRequired("resource")
	return cmd
}
