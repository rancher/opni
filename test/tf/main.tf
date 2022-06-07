resource "random_id" "test_id" {
  byte_length = 4
}

data "aws_default_tags" "current" {}

data "aws_route53_zone" "zone" {
  zone_id = var.route53_zone_id
}

locals {
  gateway_fqdn = "${random_id.test_id.hex}.opni-e2e-test.${data.aws_route53_zone.zone.name}"
  grafana_fqdn = "grafana.${random_id.test_id.hex}.opni-e2e-test.${data.aws_route53_zone.zone.name}"
}

resource "aws_acm_certificate" "cert" {
  domain_name               = local.grafana_fqdn
  subject_alternative_names = []
  validation_method         = "DNS"
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation_records" {
  count   = 1
  name    = aws_acm_certificate.cert.domain_validation_options.*.resource_record_name[count.index]
  records = [aws_acm_certificate.cert.domain_validation_options.*.resource_record_value[count.index]]
  type    = aws_acm_certificate.cert.domain_validation_options.*.resource_record_type[count.index]
  ttl     = 60
  zone_id = data.aws_route53_zone.zone.zone_id
}

resource "aws_acm_certificate_validation" "cert_validation" {
  certificate_arn         = aws_acm_certificate.cert.arn
  validation_record_fqdns = [for record in aws_route53_record.cert_validation_records : record.fqdn]
}

resource "rancher2_cloud_credential" "cc" {
  name = "opni-e2e-test-${random_id.test_id.hex}"
  amazonec2_credential_config {
    access_key = var.aws_access_key
    secret_key = var.aws_secret_key
  }
}

resource "rancher2_cluster" "opni_e2e_test_eks" {
  name = "opni-e2e-test-eks-${random_id.test_id.hex}"
  eks_config_v2 {
    cloud_credential_id = rancher2_cloud_credential.cc.id
    region              = "us-east-2"
    kubernetes_version  = "1.22"
    node_groups {
      name          = "opni-e2e-test-nodes-${random_id.test_id.hex}"
      instance_type = "t3.xlarge"
      desired_size  = 3
      max_size      = 3
      disk_size     = 64
      ec2_ssh_key   = var.rancher_ssh_key
      tags          = data.aws_default_tags.current.tags
      resource_tags = data.aws_default_tags.current.tags
    }
    tags           = data.aws_default_tags.current.tags
    private_access = true
    public_access  = true
  }
}

locals {
  kubeconfig = yamldecode(rancher2_cluster.opni_e2e_test_eks.kube_config)
}

resource "local_file" "kubeconfig" {
  content  = rancher2_cluster.opni_e2e_test_eks.kube_config
  filename = "${path.module}/kubeconfig.yaml"
}

resource "aws_s3_bucket" "s3_bucket" {
  bucket        = "opni-e2e-test-${random_id.test_id.hex}"
  force_destroy = true
}

resource "aws_s3_bucket_acl" "s3_bucket_acl" {
  bucket = aws_s3_bucket.s3_bucket.id
  acl    = "private"
}

resource "helm_release" "cert_manager" {
  name       = "cert-manager"
  repository = "https://charts.jetstack.io"
  chart      = "cert-manager"
  set {
    name  = "installCRDs"
    value = "true"
  }
  depends_on = [
    rancher2_cluster.opni_e2e_test_eks,
  ]
}

resource "helm_release" "nginx" {
  name       = "nginx"
  repository = "https://helm.nginx.com/stable"
  chart      = "nginx-ingress"
  depends_on = [
    local.kubeconfig,
    rancher2_cluster.opni_e2e_test_eks,
  ]
  values = [
    <<EOF
controller:
  service:
    httpsPort:
      targetPort: http
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-backend-protocol: http
      service.beta.kubernetes.io/aws-load-balancer-ssl-cert: ${aws_acm_certificate.cert.arn}
      service.beta.kubernetes.io/aws-load-balancer-ssl-ports: "*"
EOF
  ]
}

resource "helm_release" "opni_prometheus_crd" {
  name  = "opni-prometheus-crd"
  chart = "../../charts/opni-prometheus-crd/0.4.1"
}

resource "helm_release" "opni_crd" {
  name  = "opni-crd"
  chart = "../../charts/opni-crd/0.5.0"
}

resource "helm_release" "opni" {
  depends_on = [
    helm_release.opni_prometheus_crd,
    helm_release.opni_crd,
    helm_release.cert_manager,
  ]
  name             = "opni"
  namespace        = "opni"
  create_namespace = true
  chart            = "../../charts/opni/0.5.0"
  values = [yamlencode({
    image = {
      repository = var.image_repository
      tag        = var.image_tag
    }
    gateway = {
      enabled     = true
      hostname    = local.gateway_fqdn
      serviceType = "LoadBalancer"
      auth = {
        provider = "openid"
        openid = {
          discovery = {
            issuer = "https://${aws_cognito_user_pool.user_pool.endpoint}"
          }
          identifyingClaim  = "email"
          clientID          = "${aws_cognito_user_pool_client.grafana.id}"
          clientSecret      = "${aws_cognito_user_pool_client.grafana.client_secret}"
          scopes            = ["openid", "profile", "email"]
          roleAttributePath = "'\"custom:grafana_role\"'"
        }
      }
    }
    monitoring = {
      enabled = true
      grafana = {
        enabled  = true
        hostname = local.grafana_fqdn
        config = {
          log = {
            level = "info"
          }
        }
      }
      cortex = {
        storage = {
          backend = "s3"
          s3 = {
            "endpoint"         = "s3.us-east-2.amazonaws.com"
            "region"           = "us-east-2"
            "bucketName"       = aws_s3_bucket.s3_bucket.bucket
            "accessKeyID"      = var.aws_access_key
            "secretAccessKey"  = var.aws_secret_key
            "signatureVersion" = "v4"
          }
        }
      }
    }
    logging = {
      enabled = false
    }
  })]
}

resource "helm_release" "opni_agent" {
  depends_on = [
    helm_release.opni,
  ]
  name             = "opni-agent"
  namespace        = "opni-agent"
  create_namespace = true
  chart            = "../../charts/opni-agent/0.5.0"
  values = [yamlencode({
    address = "opni-monitoring.opni.svc:9090"
    image = {
      repository = var.image_repository
      tag        = var.image_tag
    }
    metrics = {
      enabled = true
    }
    bootstrapInCluster = {
      enabled           = true
      managementAddress = "opni-monitoring-internal.opni.svc:11090"
    }
    kube-prometheus-stack = {
      enabled = true
    }
  })]
}

data "kubernetes_service_v1" "opni_monitoring" {
  depends_on = [
    helm_release.opni
  ]
  metadata {
    namespace = "opni"
    name      = "opni-monitoring"
  }
}

data "kubernetes_service_v1" "nginx" {
  depends_on = [
    helm_release.opni,
    helm_release.nginx
  ]
  metadata {
    namespace = "default"
    name      = "nginx-nginx-ingress"
  }
}

resource "aws_route53_record" "gateway" {
  allow_overwrite = true
  name            = local.gateway_fqdn
  type            = "CNAME"
  zone_id         = var.route53_zone_id
  records         = [data.kubernetes_service_v1.opni_monitoring.status.0.load_balancer.0.ingress.0.hostname]
  ttl             = 60
}

resource "aws_route53_record" "grafana" {
  allow_overwrite = true
  name            = local.grafana_fqdn
  type            = "CNAME"
  zone_id         = var.route53_zone_id
  records         = [data.kubernetes_service_v1.nginx.status.0.load_balancer.0.ingress.0.hostname]
  ttl             = 60
}

resource "kubernetes_ingress_v1" "grafana_ingress" {
  depends_on = [
    data.kubernetes_service_v1.nginx,
  ]
  lifecycle {
    ignore_changes = [
      metadata[0].annotations
    ]
  }
  metadata {
    name      = "grafana"
    namespace = "opni"
    annotations = {
      "nginx.ingress.kubernetes.io/backend-protocol" = "HTTP"
    }
  }
  spec {
    ingress_class_name = "nginx"
    rule {
      host = local.grafana_fqdn
      http {
        path {
          path = "/"
          backend {
            service {
              name = "grafana-service"
              port {
                number = 3000
              }
            }
          }
        }
      }
    }
  }
  wait_for_load_balancer = true
}

resource "aws_cognito_user_pool" "user_pool" {
  name = "opni-e2e-test-${random_id.test_id.hex}"
  password_policy {
    minimum_length                   = 6
    require_lowercase                = false
    require_numbers                  = false
    require_symbols                  = false
    require_uppercase                = false
    temporary_password_validity_days = 7
  }
  auto_verified_attributes = ["email"]
  admin_create_user_config {
    allow_admin_create_user_only = true
  }
  schema {
    attribute_data_type      = "String"
    developer_only_attribute = false
    mutable                  = true
    name                     = "grafana_role"
    required                 = false
    string_attribute_constraints {
      min_length = 1
      max_length = 20
    }
  }
  username_configuration {
    case_sensitive = true
  }
}

resource "aws_cognito_user_pool_domain" "domain" {
  domain       = "opni-e2e-test-${random_id.test_id.hex}"
  user_pool_id = aws_cognito_user_pool.user_pool.id
}

resource "aws_cognito_user" "example" {
  user_pool_id = aws_cognito_user_pool.user_pool.id
  username     = "example"
  password     = "password"
  attributes = {
    "email"        = "example@example.com"
    "grafana_role" = "Admin"
  }
}

resource "aws_cognito_user_pool_client" "grafana" {
  name                                 = "grafana"
  user_pool_id                         = aws_cognito_user_pool.user_pool.id
  callback_urls                        = ["https://${local.grafana_fqdn}/login/generic_oauth"]
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  generate_secret                      = true
  supported_identity_providers         = ["COGNITO"]
}
