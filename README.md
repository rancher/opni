# opni

Opni is a collection of AIOPs tools - it currently features AI-infused log monitoring for Kubernetes.
____
#### What does it give me?
* Insights into logs from your cluster's workloads, control plane & etcd
* Opni insights dashboard to inspect logs
* Ability to send alerts (slack/email/etc) when anomaly threshold is breached

Every log message sent to Opni will be marked as either normal, suspicious, or anomalous.
If a lot of logs in a short period of time are marked as suspicious or anomalous it is probably worth investigating!
The anomaly threshold is a number that can tuned depending on your volume of logs and how frequently Opni is predicting anomalies.
____
#### Prerequisites
* Must have at least one GPU node (K80 GPU or higher) and at least two CPUs as part of the cluster with at least 10 GiB memory as well.
____
#### How does it work?
Ship logs over to your Opni cluster with [Rancher Logging](https://rancher.com/docs/rancher/v2.x/en/logging/v2.5/). That's it! Opni will continuously learn the nature of your logs and will update models automatically.
____
#### Ship Logs to Opni
TODO
____
#### Upcoming features
- [ ] Prediction feedback - give feedback for incorrect predictions so the AI adapts better to your logs
- Control plane log anomaly detection for additional Kubernetes distributions besides RKE including K3S and EKS.

## Contributing
We use `pre-commit` for formatting auto-linting and checking import. Please refer to [installation](https://pre-commit.com/#installation) to install the pre-commit or run `pip install pre-commit`. Then you can activate it for this repo. Once it's activated, it will lint and format the code when you make a git commit. It makes changes in place. If the code is modified during the reformatting, it needs to be staged manually.

```
# Install
pip install pre-commit

# Install the git commit hook to invoke automatically every time you do "git commit"
pre-commit install

# (Optional)Manually run against all files
pre-commit run --all-files
```
