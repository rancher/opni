package aws

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/networking/v1"

	. "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pkg/errors"
)

func (p *provisioner) buildNamespace(ctx *Context, provider ProviderResource) (*corev1.Namespace, error) {
	opniNs, err := corev1.NewNamespace(ctx, "opni", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: String("opni"),
		},
	}, Provider(provider), RetainOnDelete(true))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return opniNs, nil
}

func (p *provisioner) buildCertManager(ctx *Context, provider ProviderResource) error {
	_, err := helm.NewRelease(ctx, "cert-manager", &helm.ReleaseArgs{
		Name:            String("cert-manager"),
		Namespace:       String("cert-manager"),
		CreateNamespace: Bool(true),
		RepositoryOpts: &helm.RepositoryOptsArgs{
			Repo: String("https://charts.jetstack.io"),
		},
		Chart: String("cert-manager"),
		Values: Map{
			"installCRDs": String("true"),
		},
		WaitForJobs: Bool(true),
	}, Provider(provider), RetainOnDelete(true))
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *provisioner) buildNginx(ctx *Context, provider ProviderResource) (loadBalancer StringOutput, err error) {
	nginxChart, err := helm.NewRelease(ctx, "nginx", &helm.ReleaseArgs{
		Name:            String("nginx"),
		Namespace:       String("nginx"),
		CreateNamespace: Bool(true),
		RepositoryOpts: &helm.RepositoryOptsArgs{
			Repo: String("https://helm.nginx.com/stable"),
		},
		Chart: String("nginx-ingress"),
		Values: Map{
			"controller": Map{
				"service": Map{
					"httpsPort": Map{
						"targetPort": String("http"),
					},
					"annotations": Map{
						"service.beta.kubernetes.io/aws-load-balancer-backend-protocol": String("http"),
						"service.beta.kubernetes.io/aws-load-balancer-ssl-cert":         p.dns.Cert.Arn,
						"service.beta.kubernetes.io/aws-load-balancer-ssl-ports":        String("*"),
					},
				},
			},
		},
		WaitForJobs: Bool(true),
	}, Provider(provider), RetainOnDelete(true))
	if err != nil {
		return StringOutput{}, errors.WithStack(err)
	}
	hostname := All(nginxChart.Status.Namespace(), nginxChart.Status.Name()).
		ApplyT(func(args []any) (StringOutput, error) {
			namespace := args[0].(*string)
			name := args[1].(*string)

			nginxLBSvc, err := corev1.GetService(ctx, "nginx-ingress", ID(
				fmt.Sprintf("%s/%s-nginx-ingress", *namespace, *name),
			), nil, Provider(provider), Parent(nginxChart))
			if err != nil {
				return StringOutput{}, errors.WithStack(err)
			}
			return nginxLBSvc.Status.LoadBalancer().
				Ingress().Index(Int(0)).Hostname().Elem(), nil
		}).(StringOutput)
	return hostname, nil
}

func (p *provisioner) buildGrafanaIngress(ctx *Context, namespace StringPtrInput, provider ProviderResource) error {
	_, err := networkingv1.NewIngress(ctx, "grafana", &networkingv1.IngressArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      String("grafana"),
			Namespace: namespace,
			Annotations: StringMap{
				"nginx.ingress.kubernetes.io/backend-protocol": String("HTTP"),
			},
		},
		Spec: &networkingv1.IngressSpecArgs{
			IngressClassName: String("nginx"),
			Rules: networkingv1.IngressRuleArray{
				networkingv1.IngressRuleArgs{
					Host: p.dns.GrafanaFqdn,
					Http: &networkingv1.HTTPIngressRuleValueArgs{
						Paths: networkingv1.HTTPIngressPathArray{
							networkingv1.HTTPIngressPathArgs{
								Path:     String("/"),
								PathType: String("Prefix"),
								Backend: &networkingv1.IngressBackendArgs{
									Service: networkingv1.IngressServiceBackendArgs{
										Name: String("grafana-service"),
										Port: networkingv1.ServiceBackendPortArgs{
											Number: Int(3000),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}, Provider(provider), RetainOnDelete(true))
	return err
}
