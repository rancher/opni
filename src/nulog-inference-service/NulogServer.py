# Standard Library
import logging
import os
from collections import defaultdict
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
MAX_DICT_SIZE = 10000
MIN_LOG_TOKENS = int(os.getenv("MIN_LOG_TOKENS", 1))


class NulogServer:
    def __init__(self):
        self.is_ready = False
        self.parser = None
        self.saved_preds = defaultdict(float)

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

    def load(self, save_path="output/"):
        if using_GPU:
            logging.debug("inferencing with GPU.")
        else:
            logging.debug("inferencing without GPU.")
        try:
            self.parser = nuloginf.init_model(save_path=save_path)
            self.is_ready = True
            logging.info("Nulog model gets loaded.")
        except Exception as e:
            logging.error("No Nulog model currently {}".format(e))

    def predict(self, logs: List[str]):
        """
        logs: masked logs
        """
        if not self.is_ready:
            logging.warning("Warning: NuLog model is not ready yet!")
            return None
        if len(self.saved_preds) > MAX_DICT_SIZE:
            self.saved_preds.clear()

        # output = nuloginf.predict(self.parser, logs)
        output = []
        for log in logs:
            tokens = self.parser.tokenize_data([log], isTrain=False)
            if len(tokens[0]) < MIN_LOG_TOKENS:
                output.append(1)
            elif log in self.saved_preds:
                output.append(self.saved_preds[log])
            else:
                pred = (self.parser.predict(tokens))[0]
                output.append(pred)
                self.saved_preds[log] = pred
        logging.debug("size of saved preds : {}".format(len(self.saved_preds)))
        return output
