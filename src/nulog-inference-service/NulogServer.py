# Standard Library
import logging
import os
import time
from typing import List

# Third Party
import boto3
import inference as nuloginf
from botocore.config import Config
from NuLogParser import using_GPU

MINIO_ACCESS_KEY = os.environ["MINIO_ACCESS_KEY"]
MINIO_SECRET_KEY = os.environ["MINIO_SECRET_KEY"]


class NulogServer:
    def __init__(self, method: str = "predict"):
        self.method = method
        self.parser = None
        self.download_from_minio()
        self.load()
        self.is_ready = False

    def download_from_minio(
        self,
        decoded_payload={
            "bucket": "nulog-models",
            "bucket_files": {
                "model_file": "nulog_model_latest.pt",
                "vocab_file": "vocab.txt",
            },
        },
    ):

        endpoint_url = "http://minio.default.svc.cluster.local:9000"
        minio_client = boto3.resource(
            "s3",
            endpoint_url=endpoint_url,
            aws_access_key_id=MINIO_ACCESS_KEY,
            aws_secret_access_key=MINIO_SECRET_KEY,
            config=Config(signature_version="s3v4"),
        )
        if not os.path.exists("output/"):
            os.makedirs("output")

        bucket_name = decoded_payload["bucket"]
        bucket_files = decoded_payload["bucket_files"]
        for k in bucket_files:
            try:
                minio_client.meta.client.download_file(
                    bucket_name, bucket_files[k], "output/{}".format(bucket_files[k])
                )
            except Exception as e:
                logging.error(
                    "Cannot currently obtain necessary model files. Exiting function"
                )
                return

    def load(self):

        if using_GPU:
            logging.info("inferencing with GPU.")
        else:
            logging.info("inferencing without GPU.")
        try:
            self.parser = nuloginf.init_model()
            logging.info("Nulog model gets loaded.")
            self.is_ready = True
        except Exception as e:
            logging.error("No Nulog model currently {}".format(e))
        # self.predict(test_texts)

    def predict(self, logs: List[str], feature_names=None):
        if not self.is_ready:
            logging.warning("Warning: NuLog model is not ready yet!")
            return None
        start_time = time.time()
        output = nuloginf.predict(self.parser, logs)
        logging.info(
            (
                "--- predict %s logs in %s seconds ---"
                % (len(logs), time.time() - start_time)
            )
        )
        return output
