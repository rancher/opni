package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	. "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"github.com/rancher/opni/infra/pkg/aws"
	"github.com/rancher/opni/infra/pkg/resources"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
)

func main() {
	Run(run)
}

type InfraConfig struct {
	NamePrefix           string   `json:"namePrefix"`
	VPCBlock             string   `json:"vpcBlock"`
	SubnetBlocks         []string `json:"subnetBlocks"`
	NodeInstanceType     string   `json:"nodeInstanceType"`
	NodeGroupMinSize     int      `json:"nodeGroupMinSize"`
	NodeGroupMaxSize     int      `json:"nodeGroupMaxSize"`
	NodeGroupDesiredSize int      `json:"nodeGroupDesiredSize"`
	ZoneID               string   `json:"zoneID"`
}

type TestConfig struct {
	Cloud     string `json:"cloud"`
	ImageRepo string `json:"imageRepo"`
	ImageTag  string `json:"imageTag"`
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

	var infraConfig InfraConfig
	var testConfig TestConfig
	config.RequireObject(ctx, "infra", &infraConfig)
	config.GetObject(ctx, "test", &testConfig)
	if value, ok := os.LookupEnv("CLOUD"); ok {
		testConfig.Cloud = value
	}
	if value, ok := os.LookupEnv("IMAGE_REPO"); ok {
		testConfig.ImageRepo = value
	}
	if value, ok := os.LookupEnv("IMAGE_TAG"); ok {
		testConfig.ImageTag = value
	}

	var provisioner resources.Provisioner

	switch testConfig.Cloud {
	case "aws":
		provisioner = aws.NewProvisioner()
	default:
		return errors.Errorf("unsupported cloud: %s", testConfig.Cloud)
	}

	var id StringOutput
	if rand, err := random.NewRandomId(ctx, "id", &random.RandomIdArgs{
		ByteLength: Int(4),
	}); err != nil {
		return errors.WithStack(err)
	} else {
		id = rand.Hex
	}

	conf := resources.MainClusterConfig{
		ID:                   id,
		NamePrefix:           infraConfig.NamePrefix,
		NodeInstanceType:     infraConfig.NodeInstanceType,
		NodeGroupMinSize:     infraConfig.NodeGroupMinSize,
		NodeGroupMaxSize:     infraConfig.NodeGroupMaxSize,
		NodeGroupDesiredSize: infraConfig.NodeGroupDesiredSize,
		ZoneID:               infraConfig.ZoneID,
	}

	mainCluster, err := provisioner.ProvisionMainCluster(ctx, conf)
	if err != nil {
		return err
	}

	opniServiceLB := mainCluster.Provider.ApplyT(func(k *kubernetes.Provider) (StringOutput, error) {
		opniPrometheusCrd, err := helm.NewRelease(ctx, "opni-prometheus-crd", &helm.ReleaseArgs{
			Name:        String("opni-prometheus-crd"),
			Namespace:   String("opni"),
			Chart:       String("../charts/opni-prometheus-crd/0.4.1"),
			Atomic:      Bool(true),
			ForceUpdate: Bool(true),
			Timeout:     Int(60),
		}, Provider(k))
		if err != nil {
			return StringOutput{}, errors.WithStack(err)
		}

		opniCrd, err := helm.NewRelease(ctx, "opni-crd", &helm.ReleaseArgs{
			Name:        String("opni-crd"),
			Namespace:   String("opni"),
			Chart:       String("../charts/opni-crd/0.5.0"),
			Atomic:      Bool(true),
			ForceUpdate: Bool(true),
			Timeout:     Int(60),
		}, Provider(k), DependsOn([]Resource{opniPrometheusCrd}))
		if err != nil {
			return StringOutput{}, errors.WithStack(err)
		}

		opni, err := helm.NewRelease(ctx, "opni", &helm.ReleaseArgs{
			Chart:     String("../charts/opni/0.5.0"),
			Name:      String("opni"),
			Namespace: String("opni"),
			Values: Map{
				"image": Map{
					"repository": String(testConfig.ImageRepo),
					"tag":        String(testConfig.ImageTag),
				},
				"gateway": Map{
					"enabled":     Bool(true),
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
				"monitoring": Map{
					"enabled": Bool(true),
					"grafana": Map{
						"enabled":  Bool(true),
						"hostname": mainCluster.GrafanaHostname,
					},
					"cortex": Map{
						"storage": Map{
							"backend": String("s3"),
							"s3": Map{
								"endpoint":         mainCluster.S3.Endpoint,
								"region":           mainCluster.S3.Region,
								"bucketName":       mainCluster.S3.Bucket,
								"accessKeyID":      mainCluster.S3.AccessKeyID,
								"secretAccessKey":  mainCluster.S3.SecretAccessKey,
								"signatureVersion": String("v4"),
							},
						},
					},
				},
			},
			Atomic:      Bool(true),
			ForceUpdate: Bool(true),
			Timeout:     Int(300),
		}, Provider(k), DependsOn([]Resource{opniPrometheusCrd, opniCrd}))
		if err != nil {
			return StringOutput{}, errors.WithStack(err)
		}

		_, err = helm.NewRelease(ctx, "opni-agent", &helm.ReleaseArgs{
			Chart:           String("../charts/opni-agent/0.5.0"),
			Name:            String("opni-agent"),
			Namespace:       String("opni"),
			CreateNamespace: Bool(true),
			Values: Map{
				"address": String("opni-monitoring.opni.svc:9090"),
				"image": Map{
					"repository": String(testConfig.ImageRepo),
					"tag":        String(testConfig.ImageTag),
				},
				"metrics": Map{
					"enabled": Bool(true),
				},
				"bootstrapInCluster": Map{
					"enabled":           Bool(true),
					"managementAddress": String("opni-monitoring-internal.opni.svc:11090"),
				},
				"kube-prometheus-stack": Map{
					"enabled": Bool(true),
				},
			},
			Atomic:      Bool(true),
			ForceUpdate: Bool(true),
			Timeout:     Int(300),
		}, Provider(k), DependsOn([]Resource{opniPrometheusCrd, opniCrd, opni}))
		if err != nil {
			return StringOutput{}, errors.WithStack(err)
		}

		opniServiceLB := All(opni.Status.Namespace(), opni.Status.Name()).
			ApplyT(func(args []any) (StringOutput, error) {
				timeout, ca := context.WithTimeout(context.Background(), 5*time.Minute)
				defer ca()
				namespace := args[0].(*string)
				var opniLBSvc *corev1.Service
				for timeout.Err() == nil {
					opniLBSvc, err = corev1.GetService(ctx, "opni-monitoring", ID(
						fmt.Sprintf("%s/opni-monitoring", *namespace),
					), nil, Provider(k), Parent(opni), Timeouts(&CustomTimeouts{
						Create: (5 * time.Minute).String(),
						Update: (5 * time.Minute).String(),
					}))
					if err != nil {
						time.Sleep(time.Second * 1)
						continue
					}
					break
				}
				if timeout.Err() != nil {
					return StringOutput{}, errors.WithStack(timeout.Err())
				}
				return opniLBSvc.Status.LoadBalancer().
					Ingress().Index(Int(0)).Hostname().Elem(), nil
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

	ctx.Export("kubeconfig", mainCluster.Kubeconfig)
	ctx.Export("gateway_url", mainCluster.GatewayHostname)
	ctx.Export("grafana_url", mainCluster.GrafanaHostname.ApplyT(func(hostname string) string {
		return fmt.Sprintf("https://%s", hostname)
	}).(StringOutput))
	ctx.Export("s3_bucket", mainCluster.S3.Bucket)
	ctx.Export("s3_endpoint", mainCluster.S3.Endpoint)
	ctx.Export("oauth_client_id", mainCluster.OAuth.ClientID)
	ctx.Export("oauth_client_secret", mainCluster.OAuth.ClientSecret)
	ctx.Export("oauth_issuer_url", mainCluster.OAuth.Issuer)
	return nil
}
