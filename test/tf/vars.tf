variable "rancher_api_url" {
  type      = string
  sensitive = true
}

variable "rancher_api_token" {
  type      = string
  sensitive = true
}

variable "rancher_ssh_key" {
  type      = string
  sensitive = true
}

variable "aws_access_key" {
  type      = string
  sensitive = true
}

variable "aws_secret_key" {
  type      = string
  sensitive = true
}

variable "route53_zone_id" {
  type      = string
  sensitive = true
}

variable "image_tag" {
  type    = string
  default = "main"
}

variable "image_repository" {
  type    = string
  default = "rancher/opni"
}
