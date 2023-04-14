# Opni Metric Analysis Service

* This service offers a new feature to match metrics to 2 types of known pre-defined anomaly chart patterns. For each time of the patterns, a grafana dashboard that visualize all anomaly metrics that match to the patterns will be dynamically generated for user to consume. This feature will make operators' life easier in the process of outage diagnosis.

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
