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
MINIO_ENDPOINT = os.environ["MINIO_ENDPOINT"]
DEFAULT_MODELREADY_PAYLOAD = {
    "bucket": "nulog-models",
    "bucket_files": {
        "model_file": "nulog_model_latest.pt",
        "vocab_file": "vocab.txt",
    },
}


class NulogServer:
    def __init__(self):
        self.is_ready = False
        self.parser = None
        self.download_from_minio()
        self.load()

    def download_from_minio(
        self,
        decoded_payload: dict = DEFAULT_MODELREADY_PAYLOAD,
    ):
        minio_client = boto3.resource(
            "s3",
            endpoint_url=MINIO_ENDPOINT,
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
            logging.debug("inferencing with GPU.")
        else:
            logging.debug("inferencing without GPU.")
        try:
            self.parser = nuloginf.init_model()
            self.is_ready = True
            logging.info("Nulog model gets loaded.")
        except Exception as e:
            logging.error("No Nulog model currently {}".format(e))

    def predict(self, logs: List[str]):
        if not self.is_ready:
            logging.warning("Warning: NuLog model is not ready yet!")
            return None
        start_time = time.time()
        output = nuloginf.predict(self.parser, logs)
        logging.debug(
            (
                "--- predict %s logs in %s seconds ---"
                % (len(logs), time.time() - start_time)
            )
        )
        return output
