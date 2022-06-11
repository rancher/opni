package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/rancher/opni/infra/pkg/resources"

	. "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/acm"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cognito"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
)

func NewProvisioner() resources.Provisioner {
	return &provisioner{}
}

type provisioner struct {
	clusterName StringOutput
	eks         *eksResources
	dns         *dnsResources
	s3          *s3Resources
	cognito     *cognitoResources
}

type eksResources struct {
	Cluster *eks.Cluster
}

type dnsResources struct {
	GrafanaFqdn StringOutput
	GatewayFqdn StringOutput
	Cert        *acm.Certificate
}

type s3Resources struct {
	Bucket *s3.Bucket
}

type cognitoResources struct {
	UserPool *cognito.UserPool
	Client   *cognito.UserPoolClient
}

func (p *provisioner) ProvisionMainCluster(ctx *pulumi.Context, conf resources.MainClusterConfig) (*resources.MainClusterOutput, error) {
	p.clusterName = conf.ID.ApplyT(func(id string) string {
		return fmt.Sprintf("opni-e2e-test-%s", id)
	}).(StringOutput)

	eks, err := p.buildEksResources(ctx, conf)
	if err != nil {
		return nil, err
	}

	if p.dns, err = p.buildDnsResources(ctx, conf); err != nil {
		return nil, err
	}

	if p.s3, err = p.buildS3Resources(ctx, conf); err != nil {
		return nil, err
	}

	if p.cognito, err = p.buildCognitoResources(ctx, conf); err != nil {
		return nil, err
	}

	lbHostname := eks.Cluster.Provider.ApplyT(func(k *kubernetes.Provider) (StringOutput, error) {
		namespace, err := p.buildNamespace(ctx, k)
		if err != nil {
			return StringOutput{}, err
		}

		if err := p.buildCertManager(ctx, k); err != nil {
			return StringOutput{}, err
		}

		lbHostname, err := p.buildNginx(ctx, k)
		if err != nil {
			return StringOutput{}, err
		}

		if err := p.buildGrafanaIngress(ctx, namespace.Metadata.Name(), k); err != nil {
			return StringOutput{}, err
		}

		return lbHostname, nil
	}).(StringOutput)

	creds, err := currentAwsCreds()
	if err != nil {
		return nil, err
	}

	return &resources.MainClusterOutput{
		Provider:             eks.Cluster.Provider,
		Kubeconfig:           StringOutput(eks.Cluster.Kubeconfig),
		GrafanaHostname:      p.dns.GrafanaFqdn,
		GatewayHostname:      p.dns.GatewayFqdn,
		LoadBalancerHostname: lbHostname,
		OAuth: resources.OAuthOutput{
			Issuer: p.cognito.UserPool.Endpoint.ApplyT(func(str string) string {
				return fmt.Sprintf("https://%s", str)
			}).(StringOutput),
			ClientID:     p.cognito.Client.ID(),
			ClientSecret: p.cognito.Client.ClientSecret,
		},
		S3: resources.S3Output{
			Endpoint: p.s3.Bucket.Region.ApplyT(func(region string) string {
				return fmt.Sprintf("s3.%s.amazonaws.com", region)
			}).(StringOutput),
			Region:          p.s3.Bucket.Region,
			Bucket:          p.s3.Bucket.Bucket,
			AccessKeyID:     String(creds.AccessKeyID),
			SecretAccessKey: String(creds.SecretAccessKey),
		},
	}, nil
}

func (p *provisioner) ProvisionDNSRecord(ctx *Context, name string, conf resources.DNSRecordConfig) (StringOutput, error) {
	record, err := route53.NewRecord(ctx, name, &route53.RecordArgs{
		AllowOverwrite: Bool(true),
		Name:           conf.Name,
		Type:           String(conf.Type),
		ZoneId:         String(conf.ZoneID),
		Records:        conf.Records,

		Ttl: IntPtr(60),
	})
	if err != nil {
		return StringOutput{}, errors.WithStack(err)
	}
	return record.Fqdn, nil
}

func currentAwsCreds() (*credentials.Value, error) {
	session := session.Must(session.NewSession())
	creds, err := session.Config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, errors.New("missing access key or secret key")
	}
	return &creds, nil
}
