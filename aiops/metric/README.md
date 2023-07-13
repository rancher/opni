# Opni Metric Analysis Service

* This service offers a new feature to match metrics to 2 types of known pre-defined anomaly chart patterns. For each time of the patterns, a grafana dashboard that visualize all anomaly metrics that match to the patterns will be dynamically generated for user to consume. This feature will make operators' life easier in the process of outage diagnosis. 

* The 2 types of pre-defined patterns: <img width="1401" alt="220768433-82056fd1-494c-4afe-9822-77b0566fd99d" src="https://github.com/rancher/opni/assets/4568163/221bc7b7-2ec1-447a-9f2d-c7e2507e6f4a">

* This [OEP](https://github.com/rancher/opni/blob/main/enhancements/aiops/20230120-Identify-metrics-that-match-known-anomaly-chart-patterns.md) provides more design and technical details.

## Get Started

### Generate proto files for the python service
run `mage generate` in the root dir of opni to generate .py proto-scripts from cortexadmin.proto 
To edit the python code generator rule: the file - https://github.com/kralicky/ragu/tree/main/pkg/plugins/python
1. clone `github.com/kralicky/ragu` to somewhere
2. run go mod edit -replace github.com/kralicky/ragu => /path/to/clone && go mod tidy
3. mage generate again

### Build image and create a deployment on k8s
Build image:
```
cd ../..
dagger-cue do aiopsload
docker tag rancher/metric-ai-service  tybalex/metric-ai-service  &&  docker push tybalex/metric-ai-service
```

Run this to create a deployment on k8s:
```
kubectl apply -f deploy.yaml
```


## Testing
Install libs for testing
```
pip install -r requirements.txt
pip install -r test-requirements.txt
```

Run pytest with coverage report on the src dir `metric_analysis`
```
pytest --cov metric_analysis
```

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
