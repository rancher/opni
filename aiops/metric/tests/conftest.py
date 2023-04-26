# Standard Library
import json
import os

# Third Party
import pytest

os.environ["NATS_SERVER_URL"] = ""
os.environ["NATS_USERNAME"] = ""
os.environ["NATS_PASSWORD"] = ""
os.environ["CNN_MODEL_PATH"] = "metric_analysis/model/model.pth"
