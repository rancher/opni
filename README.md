![](./docs/content/en/static/logo-light.svg)

[![Build](https://github.com/rancher/opni-monitoring/actions/workflows/build.yaml/badge.svg)](https://github.com/rancher/opni-monitoring/actions/workflows/build.yaml)
[![codecov](https://codecov.io/gh/rancher/opni-monitoring/branch/main/graph/badge.svg?token=EAJW6K3HXP)](https://codecov.io/gh/rancher/opni-monitoring)
[![Go Report Card](https://goreportcard.com/badge/github.com/rancher/opni-monitoring)](https://goreportcard.com/report/github.com/rancher/opni-monitoring)
[![Maintainability](https://api.codeclimate.com/v1/badges/2284f4b5cb8fb71750ce/maintainability)](https://codeclimate.com/github/rancher/opni-monitoring/maintainability)

------

Opni Monitoring is an open-source multi-cluster monitoring system. It ingests Prometheus metrics from any number of Kubernetes clusters and provides a centralized observability plane for your infrastructure. Use Opni Monitoring to visualize metrics from all your clusters at once, and give every user their own customized view using granular access control.

## ⚡ Powered by Open-Source

Opni Monitoring is completely free Apache-licensed open-source software. It builds upon existing, ubiquitous open-source systems - [Prometheus](https://prometheus.io), [Grafana](https://grafana.com), and [Cortex](https://cortexmetrics.io) - and extends them with a number of powerful enterprise features typically only found in SaaS platforms and other proprietery solutions.

## 🔋 Batteries Included

Opni Monitoring comes out of the box with all the tools you need to get started with multi-cluster monitoring. Manage your clusters and configure access control rules with the built-in dashboard, command-line interface, or REST API. 

Opni Monitoring is secure-by-default and uses a zero-trust architecture for inter-cluster communication, with no extra setup required.

## 🔒 You Own Your Data

With Opni Monitoring, you have complete control over how and where your data is stored. Metric storage is powered by [Cortex](https://cortexmetrics.io), which provides comprehensive configuration options for data storage and retention. Several storage backends are available including S3 (cloud or self-hosted), Swift, and Kubernetes Persistent Volumes.

## Get started

Check out the [Opni Monitoring Documentation](https://rancher.github.io/opni-monitoring/) for installation guides and more.