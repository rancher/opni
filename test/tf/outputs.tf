output "kubeconfig" {
  value     = local_file.kubeconfig.content
  sensitive = true
}

output "grafana_url" {
  value = "https://${local.grafana_fqdn}"
}

output "gateway_url" {
  value = "https://${local.gateway_fqdn}"
}

output "oauth_issuer_url" {
  value = "https://${aws_cognito_user_pool.user_pool.endpoint}"
}

output "oauth_client_id" {
  value     = aws_cognito_user_pool_client.grafana.id
  sensitive = true
}

output "oauth_client_secret" {
  value     = aws_cognito_user_pool_client.grafana.client_secret
  sensitive = true
}

output "s3_bucket_url" {
  value = aws_s3_bucket.s3_bucket.bucket_regional_domain_name
}
