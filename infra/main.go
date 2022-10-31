//go:build !noinfra

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	. "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/rancher/opni/infra/pkg/aws"
	"github.com/rancher/opni/infra/pkg/resources"
	"golang.org/x/mod/semver"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
)

func main() {
	Run(run)
}

func run(ctx *Context) (runErr error) {
	defer func() {
		if sr, ok := runErr.(interface {
			StackTrace() errors.StackTrace
		}); ok {
			st := sr.StackTrace()
			ctx.Log.Error(fmt.Sprintf("%+v", st), &LogArgs{})
		}
	}()

	conf := LoadConfig(ctx)

	var provisioner resources.Provisioner

	switch conf.Cloud {
	case "aws":
		provisioner = aws.NewProvisioner()
	default:
		return errors.Errorf("unsupported cloud: %s", conf.Cloud)
	}

	var id StringOutput
	if rand, err := random.NewRandomId(ctx, "id", &random.RandomIdArgs{
		ByteLength: Int(4),
	}); err != nil {
		return errors.WithStack(err)
	} else {
		id = rand.Hex
	}

	tags := map[string]string{}
	for k, v := range conf.Tags {
		tags[k] = v
	}
	tags["PulumiProjectName"] = ctx.Project()
	tags["PulumiStackName"] = ctx.Stack()

	mainCluster, err := provisioner.ProvisionMainCluster(ctx, resources.MainClusterConfig{
		ID:                   id,
		NamePrefix:           conf.NamePrefix,
		NodeInstanceType:     conf.Cluster.NodeInstanceType,
		NodeGroupMinSize:     conf.Cluster.NodeGroupMinSize,
		NodeGroupMaxSize:     conf.Cluster.NodeGroupMaxSize,
		NodeGroupDesiredSize: conf.Cluster.NodeGroupDesiredSize,
		ZoneID:               conf.ZoneID,
		Tags:                 tags,
		UseIdInDnsNames:      conf.UseIdInDnsNames,
	})
	if err != nil {
		return err
	}
	// FIXME: ability  for agent v1
	var opniCrdChart, opniChart /*,opniAgentChart*/ string
	var chartRepoOpts *helm.RepositoryOptsArgs
	if conf.UseLocalCharts {
		var ok bool
		if opniCrdChart, ok = findLocalChartDir("opni-crd"); !ok {
			return errors.New("could not find local opni-crd chart")
		}
		if opniChart, ok = findLocalChartDir("opni"); !ok {
			return errors.New("could not find local opni chart")
		}
		// if opniAgentChart, ok = findLocalChartDir("opni-agent"); !ok {
		// 	return errors.New("could not find local opni-agent chart")
		// }
	} else {
		chartRepoOpts = &helm.RepositoryOptsArgs{
			Repo: StringPtr(conf.ChartsRepo),
		}
		opniCrdChart = "opni-crd"
		opniChart = "opni"
		// opniAgentChart = "opni-agent"
	}

	opniServiceLB := mainCluster.Provider.ApplyT(func(k *kubernetes.Provider) (StringOutput, error) {
		//FIXME
		opniCrd, err := helm.NewRelease(ctx, "opni-crd", &helm.ReleaseArgs{
			Chart:          String(opniCrdChart),
			RepositoryOpts: chartRepoOpts,
			Version:        StringPtr(conf.ChartVersion),
			Namespace:      String("opni"),
			Atomic:         Bool(true),
			ForceUpdate:    Bool(true),
			Timeout:        Int(60),
		}, Provider(k))
		if err != nil {
			return StringOutput{}, errors.WithStack(err)
		}

		opni, err := helm.NewRelease(ctx, "opni", &helm.ReleaseArgs{
			Name:           String("opni"),
			Chart:          String(opniChart),
			RepositoryOpts: chartRepoOpts,
			Version:        StringPtr(conf.ChartVersion),
			Namespace:      String("opni"),
			Values: Map{
				"image": Map{
					"repository": String(conf.ImageRepo),
					"tag":        String(conf.ImageTag),
				},
				"opni-prometheus-crd": Map{
					"enabled": Bool(false),
				},
				"gateway": Map{
					"enabled": Bool(true),
					"alerting": Map{
						"enabled": Bool(true),
					},
					"hostname":    mainCluster.GatewayHostname,
					"serviceType": String("LoadBalancer"),
					"auth": Map{
						"provider": String("openid"),
						"openid": Map{
							"discovery": Map{
								"issuer": mainCluster.OAuth.Issuer,
							},
							"identifyingClaim":  String("email"),
							"clientID":          mainCluster.OAuth.ClientID,
							"clientSecret":      mainCluster.OAuth.ClientSecret,
							"scopes":            ToStringArray([]string{"openid", "profile", "email"}),
							"roleAttributePath": String(`'"custom:grafana_role"'`),
						},
					},
				},
				"opni-agent": Map{
					"image": Map{
						"repository": String(conf.ImageRepo),
						"tag":        String(conf.MinimalImageTag),
					},
					"enabled":          Bool(true),
					"address":          String("opni"),
					"fullnameOverride": String("opni-agent"),
					"bootstrapInCluster": Map{
						"enabled": Bool(true),
					},
					"agent": Map{
						"version": String("v2"),
					},
					//FIXME change to a config value
					"kube-prometheus-stack": Map{
						"enabled": Bool(true),
					},
				},
			},
			Atomic:      Bool(true),
			ForceUpdate: Bool(true),
			Timeout:     Int(300),
			WaitForJobs: Bool(true),
		}, Provider(k), DependsOn([]Resource{opniCrd}), RetainOnDelete(true))
		if err != nil {
			return StringOutput{}, errors.WithStack(err)
		}
		//FIXME: add agent v1 back when its more stable
		// _, err = helm.NewRelease(ctx, "opni-agent", &helm.ReleaseArgs{
		// 	Chart:           String(opniAgentChart),
		// 	Version:         StringPtr(conf.ChartVersion),
		// 	RepositoryOpts:  chartRepoOpts,
		// 	Namespace:       String("opni"),
		// 	CreateNamespace: Bool(true),
		// 	Values: Map{
		// 		"address": String("opni.opni.svc:9090"),
		// 		"image": Map{
		// 			"repository": String(conf.ImageRepo),
		// 			"tag":        String(conf.ImageTag),
		// 		},
		// 		"metrics": Map{
		// 			"enabled": Bool(true),
		// 		},
		// 		"bootstrapInCluster": Map{
		// 			"enabled":           Bool(true),
		// 			"managementAddress": String("opni-internal.opni.svc:11090"),
		// 		},
		// 		"kube-prometheus-stack": Map{
		// 			"enabled": Bool(true),
		// 		},
		// 		"rules": Map{
		// 			"discovery": Map{
		// 				"prometheusRules": Map{},
		// 			},
		// 		},
		// 		"logLevel":  String("debug"),
		// 		"profiling": Bool(false), // change to true to enable profiling
		// 	},
		// 	Atomic:      Bool(true),
		// 	ForceUpdate: Bool(true),
		// 	Timeout:     Int(300),
		// }, Provider(k), DependsOn([]Resource{opniCrd, opni}), RetainOnDelete(true))
		// if err != nil {
		// 	return StringOutput{}, errors.WithStack(err)
		// }

		opniServiceLB := All(opni.Status.Namespace(), opni.Status.Name()).
			ApplyT(func(args []any) (StringOutput, error) {
				namespace := args[0].(*string)
				opniLBSvc, err := corev1.GetService(ctx, "opni", ID(
					fmt.Sprintf("%s/opni", *namespace),
				), nil, Provider(k), Parent(opni))
				if err != nil {
					return StringOutput{}, err
				}
				return opniLBSvc.Status.LoadBalancer().Ingress().Index(Int(0)).Hostname().Elem(), nil
			}).(StringOutput)
		return opniServiceLB, nil
	}).(StringOutput)

	_, err = provisioner.ProvisionDNSRecord(ctx, "gateway", resources.DNSRecordConfig{
		Name:    mainCluster.GatewayHostname,
		Type:    "CNAME",
		ZoneID:  conf.ZoneID,
		Records: StringArray{opniServiceLB},
		TTL:     60,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = provisioner.ProvisionDNSRecord(ctx, "grafana", resources.DNSRecordConfig{
		Name:    mainCluster.GrafanaHostname,
		Type:    "CNAME",
		ZoneID:  conf.ZoneID,
		Records: StringArray{mainCluster.LoadBalancerHostname},
		TTL:     60,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = provisioner.ProvisionDNSRecord(ctx, "opensearch", resources.DNSRecordConfig{
		Name:    mainCluster.OpensearchHostname,
		Type:    "CNAME",
		ZoneID:  conf.ZoneID,
		Records: StringArray{mainCluster.LoadBalancerHostname},
		TTL:     60,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	ctx.Export("kubeconfig", mainCluster.Kubeconfig.ApplyT(func(kubeconfig any) (string, error) {
		jsonData, err := json.Marshal(kubeconfig)
		if err != nil {
			return "", errors.WithStack(err)
		}
		return string(jsonData), nil
	}).(StringOutput))
	ctx.Export("gateway_url", mainCluster.GatewayHostname)
	ctx.Export("grafana_url", mainCluster.GrafanaHostname.ApplyT(func(hostname string) string {
		return fmt.Sprintf("https://%s", hostname)
	}).(StringOutput))
	ctx.Export("opensearch_url", mainCluster.OpensearchHostname.ApplyT(func(hostname string) string {
		return fmt.Sprintf("https://%s", hostname)
	}).(StringOutput))
	ctx.Export("s3_bucket", mainCluster.S3.Bucket)
	ctx.Export("s3_endpoint", mainCluster.S3.Endpoint)
	ctx.Export("s3_region", mainCluster.S3.Region)
	ctx.Export("s3_access_key_id", mainCluster.S3.AccessKeyID)
	ctx.Export("s3_secret_access_key", mainCluster.S3.SecretAccessKey)
	ctx.Export("oauth_client_id", mainCluster.OAuth.ClientID)
	ctx.Export("oauth_client_secret", mainCluster.OAuth.ClientSecret)
	ctx.Export("oauth_issuer_url", mainCluster.OAuth.Issuer)
	return nil
}

func findLocalChartDir(chartName string) (string, bool) {
	// find charts from ../charts/<chartName> and return the latest version
	dir := fmt.Sprintf("../charts/%s", chartName)
	if _, err := os.Stat(dir); err != nil {
		return "", false
	}
	versions, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	if len(versions) == 0 {
		return "", false
	}
	names := make([]string, 0, len(versions))
	for _, version := range versions {
		if version.IsDir() {
			names = append(names, version.Name())
		}
	}
	semver.Sort(names)
	return filepath.Join(dir, names[len(names)-1]), true
}
