package resources

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type MainClusterConfig struct {
	ID                   pulumi.StringOutput
	NamePrefix           string
	NodeInstanceType     string
	NodeGroupMinSize     int
	NodeGroupMaxSize     int
	NodeGroupDesiredSize int
	ZoneID               string
}

type DNSRecordConfig struct {
	Name    pulumi.StringInput
	Type    string
	ZoneID  string
	Records pulumi.StringArrayInput
	TTL     int
}

type OAuthOutput struct {
	Issuer       pulumi.StringOutput
	ClientID     pulumi.IDOutput
	ClientSecret pulumi.StringOutput
}

type S3Output struct {
	Endpoint        pulumi.StringOutput
	Region          pulumi.StringOutput
	Bucket          pulumi.StringOutput
	AccessKeyID     pulumi.StringInput
	SecretAccessKey pulumi.StringInput
}

type MainClusterOutput struct {
	Provider             kubernetes.ProviderOutput
	Kubeconfig           pulumi.StringOutput
	GrafanaHostname      pulumi.StringOutput
	GatewayHostname      pulumi.StringOutput
	LoadBalancerHostname pulumi.StringOutput
	OAuth                OAuthOutput
	S3                   S3Output
}

type Provisioner interface {
	ProvisionMainCluster(ctx *pulumi.Context, conf MainClusterConfig) (*MainClusterOutput, error)
	ProvisionDNSRecord(ctx *pulumi.Context, name string, conf DNSRecordConfig) (pulumi.StringOutput, error)
}
