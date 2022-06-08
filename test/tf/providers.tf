terraform {
  required_providers {
    rancher2 = {
      source  = "rancher/rancher2"
      version = "1.23.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.2.0"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "4.14.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "2.11.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "2.5.1"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.2.3"
    }
  }
}

provider "rancher2" {
  api_url   = var.rancher_api_url
  token_key = var.rancher_api_token
}

provider "aws" {
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
  region     = "us-east-2"

  default_tags {
    tags = {
      "TestID" = "opni-e2e-${random_id.test_id.hex}"
    }
  }
}

provider "helm" {
  kubernetes {
    host  = local.kubeconfig.clusters[0].cluster.server
    token = local.kubeconfig.users[0].user.token
  }
}

provider "kubernetes" {
  host  = local.kubeconfig.clusters[0].cluster.server
  token = local.kubeconfig.users[0].user.token
}
