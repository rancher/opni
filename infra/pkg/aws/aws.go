package aws

import (
	"fmt"
	"strings"

	"github.com/rancher/opni/infra/pkg/resources"

	. "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/acm"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cognito"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/config"
	eksapi "github.com/pulumi/pulumi-aws/sdk/v5/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-awsx/sdk/go/awsx/ec2"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
)

func (p *provisioner) buildEksResources(ctx *Context, conf resources.MainClusterConfig) (*eksResources, error) {
	vpc, err := ec2.NewVpc(ctx, conf.NamePrefix, &ec2.VpcArgs{
		Tags: ToStringMap(conf.Tags),
		NatGateways: &ec2.NatGatewayConfigurationArgs{
			Strategy: ec2.NatGatewayStrategySingle,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	cluster, err := eks.NewCluster(ctx, conf.NamePrefix, &eks.ClusterArgs{
		InstanceType:             StringPtr(conf.NodeInstanceType),
		MaxSize:                  Int(conf.NodeGroupMaxSize),
		MinSize:                  Int(conf.NodeGroupMinSize),
		DesiredCapacity:          Int(conf.NodeGroupDesiredSize),
		VpcId:                    vpc.VpcId,
		PublicSubnetIds:          vpc.PublicSubnetIds,
		PrivateSubnetIds:         vpc.PrivateSubnetIds,
		Tags:                     ToStringMap(conf.Tags),
		ClusterTags:              ToStringMap(conf.Tags),
		ClusterSecurityGroupTags: ToStringMap(conf.Tags),
		NodeSecurityGroupTags:    ToStringMap(conf.Tags),
		CreateOidcProvider:       BoolPtr(true),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	id, err := aws.GetCallerIdentity(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	rolePolicyDocument := cluster.EksCluster.Identities().Index(Int(0)).Oidcs().Index(Int(0)).Issuer().Elem().ApplyT(func(issuerUrl string) string {
		region := config.GetRegion(ctx)
		accessKey := id.AccountId

		oidcId := strings.TrimPrefix(issuerUrl, fmt.Sprintf("https://oidc.eks.%s.amazonaws.com/id/", region))
		return fmt.Sprintf(`
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::%[3]s:oidc-provider/oidc.eks.%[2]s.amazonaws.com/id/%[1]s"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "oidc.eks.%[2]s.amazonaws.com/id/%[1]s:aud": "sts.amazonaws.com",
          "oidc.eks.%[2]s.amazonaws.com/id/%[1]s:sub": "system:serviceaccount:kube-system:ebs-csi-controller-sa"
        }
      }
    }
  ]
}`[1:], oidcId, region, accessKey)
	})

	role, err := iam.NewRole(ctx, "ebs-csi-driver-role", &iam.RoleArgs{
		NamePrefix:       String("AmazonEKS_EBS_CSI_DriverRole"),
		AssumeRolePolicy: rolePolicyDocument,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = iam.NewRolePolicyAttachment(ctx, "ebs-csi-driver-role-policy-attachment", &iam.RolePolicyAttachmentArgs{
		PolicyArn: String("arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"),
		Role:      role.Name,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = eksapi.NewAddon(ctx, "aws-ebs-csi-driver", &eksapi.AddonArgs{
		AddonName:             String("aws-ebs-csi-driver"),
		AddonVersion:          String("v1.11.4-eksbuild.1"),
		ClusterName:           cluster.EksCluster.Name(),
		Tags:                  ToStringMap(conf.Tags),
		ServiceAccountRoleArn: role.Arn,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &eksResources{
		Cluster: cluster,
	}, nil
}

func (p *provisioner) buildDnsResources(ctx *Context, conf resources.MainClusterConfig) (*dnsResources, error) {
	zone, err := route53.LookupZone(ctx, &route53.LookupZoneArgs{
		ZoneId: &conf.ZoneID,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	grafanaFqdn := All(conf.ID, zone.Name).ApplyT(func(idZoneName []any) string {
		if conf.UseIdInDnsNames {
			return fmt.Sprintf("grafana.%s.%s.%s", idZoneName[0], conf.NamePrefix, idZoneName[1])
		}
		return fmt.Sprintf("grafana.%s.%s", conf.NamePrefix, idZoneName[1])
	}).(StringOutput)
	opensearchFqdn := All(conf.ID, zone.Name).ApplyT(func(idZoneName []any) string {
		if conf.UseIdInDnsNames {
			return fmt.Sprintf("opensearch.%s.%s.%s", idZoneName[0], conf.NamePrefix, idZoneName[1])
		}
		return fmt.Sprintf("opensearch.%s.%s", conf.NamePrefix, idZoneName[1])
	}).(StringOutput)
	gatewayFqdn := All(conf.ID, zone.Name).ApplyT(func(idZoneName []any) string {
		if conf.UseIdInDnsNames {
			return fmt.Sprintf("%s.%s.%s", idZoneName[0], conf.NamePrefix, idZoneName[1])
		}
		return fmt.Sprintf("%s.%s", conf.NamePrefix, idZoneName[1])
	}).(StringOutput)

	cert, err := acm.NewCertificate(ctx, "cert", &acm.CertificateArgs{
		DomainName: grafanaFqdn,
		SubjectAlternativeNames: StringArray{
			opensearchFqdn,
		},
		ValidationMethod: StringPtr("DNS"),
		Tags:             ToStringMap(conf.Tags),
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	grafanaValidationRecord, err := route53.NewRecord(ctx, "validation-grafana", &route53.RecordArgs{
		Name: cert.DomainValidationOptions.Index(Int(0)).ResourceRecordName().Elem(),
		Records: StringArray{
			cert.DomainValidationOptions.Index(Int(0)).ResourceRecordValue().Elem(),
		},
		Type:   cert.DomainValidationOptions.Index(Int(0)).ResourceRecordType().Elem(),
		Ttl:    Int(60),
		ZoneId: String(zone.Id),
	}, Parent(cert))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	opensearchValidationRecord, err := route53.NewRecord(ctx, "validation-opensearch", &route53.RecordArgs{
		Name: cert.DomainValidationOptions.Index(Int(1)).ResourceRecordName().Elem(),
		Records: StringArray{
			cert.DomainValidationOptions.Index(Int(1)).ResourceRecordValue().Elem(),
		},
		Type:   cert.DomainValidationOptions.Index(Int(1)).ResourceRecordType().Elem(),
		Ttl:    Int(60),
		ZoneId: String(zone.Id),
	}, Parent(cert))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = acm.NewCertificateValidation(ctx, "validation", &acm.CertificateValidationArgs{
		CertificateArn: cert.Arn,
		ValidationRecordFqdns: StringArray{
			grafanaValidationRecord.Fqdn,
			opensearchValidationRecord.Fqdn,
		},
	}, Parent(cert))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &dnsResources{
		GrafanaFqdn:    grafanaFqdn,
		GatewayFqdn:    gatewayFqdn,
		OpensearchFqdn: opensearchFqdn,
		Cert:           cert,
	}, nil
}

func (p *provisioner) buildS3Resources(ctx *Context, conf resources.MainClusterConfig) (*s3Resources, error) {
	s3Bucket, err := s3.NewBucket(ctx, "s3-bucket", &s3.BucketArgs{
		Bucket:       p.clusterName,
		ForceDestroy: Bool(true),
		Tags:         ToStringMap(conf.Tags),
	}, IgnoreChanges([]string{"serverSideEncryptionConfiguration"}))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	_, err = s3.NewBucketAclV2(ctx, "s3-bucket-acl", &s3.BucketAclV2Args{
		Bucket: s3Bucket.ID(),
		Acl:    String("private"),
	}, Parent(s3Bucket))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &s3Resources{
		Bucket: s3Bucket,
	}, nil
}

func (p *provisioner) buildCognitoResources(ctx *Context, conf resources.MainClusterConfig) (*cognitoResources, error) {
	userPool, err := cognito.NewUserPool(ctx, "user-pool", &cognito.UserPoolArgs{
		Name: p.clusterName,
		Tags: ToStringMap(conf.Tags),
		PasswordPolicy: cognito.UserPoolPasswordPolicyArgs{
			MinimumLength:                 Int(6),
			RequireLowercase:              Bool(false),
			RequireUppercase:              Bool(false),
			RequireNumbers:                Bool(false),
			RequireSymbols:                Bool(false),
			TemporaryPasswordValidityDays: Int(7),
		},
		AutoVerifiedAttributes: ToStringArray([]string{"email"}),
		AdminCreateUserConfig: cognito.UserPoolAdminCreateUserConfigArgs{
			AllowAdminCreateUserOnly: Bool(true),
		},
		Schemas: cognito.UserPoolSchemaArray{
			cognito.UserPoolSchemaArgs{
				AttributeDataType:      String("String"),
				DeveloperOnlyAttribute: Bool(false),
				Mutable:                Bool(true),
				Name:                   String("grafana_role"),
				Required:               Bool(false),
				StringAttributeConstraints: cognito.UserPoolSchemaStringAttributeConstraintsArgs{
					MinLength: StringPtr("1"),
					MaxLength: StringPtr("20"),
				},
			},
		},
		UsernameConfiguration: cognito.UserPoolUsernameConfigurationArgs{
			CaseSensitive: Bool(true),
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = cognito.NewUserPoolDomain(ctx, "domain", &cognito.UserPoolDomainArgs{
		Domain:     p.clusterName,
		UserPoolId: userPool.ID(),
	}, Parent(userPool))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	_, err = cognito.NewUser(ctx, "test-user", &cognito.UserArgs{
		UserPoolId: userPool.ID(),
		Username:   String("test"),
		Password:   StringPtr("password"),
		Attributes: ToStringMap(map[string]string{
			"email":        "test@example.com",
			"grafana_role": "Admin",
		}),
	}, Parent(userPool))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	userPoolClient, err := cognito.NewUserPoolClient(ctx, "grafana", &cognito.UserPoolClientArgs{
		Name:       String("grafana"),
		UserPoolId: userPool.ID(),
		CallbackUrls: StringArray{
			p.dns.GrafanaFqdn.ApplyT(func(fqdn string) string {
				return fmt.Sprintf("https://%s/login/generic_oauth", fqdn)
			}).(StringOutput),
		},
		AllowedOauthFlowsUserPoolClient: Bool(true),
		AllowedOauthFlows:               ToStringArray([]string{"code"}),
		AllowedOauthScopes:              ToStringArray([]string{"openid", "email", "profile"}),
		GenerateSecret:                  Bool(true),
		SupportedIdentityProviders:      ToStringArray([]string{"COGNITO"}),
	}, Parent(userPool))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &cognitoResources{
		UserPool: userPool,
		Client:   userPoolClient,
	}, nil
}
