# Contributing to Opni

Welcome to the Opni contributing guide. The following instructions should help you get started with Opni development.

## 1. Prerequisites

### Development Environment

Linux is the only supported OS. Mac users should use a Linux VM, EC2 instance, codespace, or similar for development.

VSCode, Goland, and Neovim are all good choices for an editor. Our team uses all three; it is mostly personal preference.

### Install Mage

We use `mage` as a build tool. for more information and download instructions visit https://magefile.org.

### Install Dagger

For CI, we use a tool called Dagger, which is a programmable CI system that lets you develop your pipelines as code. Dagger is used for all CI tasks, including building/deploying container images, helm charts, and more. For more information and download instructions visit https://docs.dagger.io.

## 2. Building Opni Locally

### Using Mage

To list available targets and their descriptions, run `mage -l`. You can run `mage` with no arguments to run the default target (generate + build) or you can run `mage <target>` to run a specific target.

## Using Dagger

Once Dagger is installed, run `mage dagger:help` (or `go run ./dagger --help`) for more info about how to configure and run dagger pipelines.

## 3. Test Environment

All of the main Opni components can be run locally for development and testing. The test fixture we use in our integration tests is available as a standalone binary which runs the Opni components (gateway, agents, cortex, prometheus, otel collector, etc.) locally and/or in-process. The test environment can be started by running `mage test:env`. Once it starts, it will display a list of available commands that can be triggered using keyboard shortcuts.

## Getting Help

If you encounter any problems or have any questions about contributing to Opni, don't hesitate to create a new issue or reach out to us on the [Rancher Users Slack](https://slack.rancher.io).
