# Standard Library
import asyncio
import json
import logging
import os
import shutil

# Third Party
import boto3
import botocore
from botocore.client import Config
from nats_wrapper import NatsWrapper
from NuLogParser import LogParser

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")

MINIO_SERVER_URL = os.environ["MINIO_SERVER_URL"]
MINIO_ACCESS_KEY = os.environ["MINIO_ACCESS_KEY"]
MINIO_SECRET_KEY = os.environ["MINIO_SECRET_KEY"]
NATS_SERVER_URL = os.environ["NATS_SERVER_URL"]

nw = NatsWrapper()


def train_nulog_model(minio_client, windows_folder_path):
    nr_epochs = 2
    num_samples = 0
    parser = LogParser()
    texts = parser.load_data(windows_folder_path)
    logging.info("NuLog model being trained from scratch")
    tokenized = parser.tokenize_data(texts, isTrain=True)
    parser.tokenizer.save_vocab()
    logging.info("vocab has been saved!")
    parser.train(tokenized, nr_epochs=nr_epochs, num_samples=num_samples)
    all_files = os.listdir("output/")
    if "nulog_model_latest.pt" in all_files and "vocab.txt" in all_files:
        minio_client.meta.client.upload_file(
            "output/nulog_model_latest.pt", "nulog-models", "nulog_model_latest.pt"
        )
        minio_client.meta.client.upload_file(
            "output/vocab.txt", "nulog-models", "vocab.txt"
        )
        logging.info("Nulog model and vocab have been uploaded to Minio.")
    else:
        logging.info("Nulog model was not able to be trained and saved successfully.")


async def send_signal_to_inference(loop):
    nulog_payload = {
        "bucket": "nulog-models",
        "bucket_files": {
            "model_file": "nulog_model_latest.pt",
            "vocab_file": "vocab.txt",
        },
    }
    encoded_nulog_json = json.dumps(nulog_payload).encode()
    await nw.connect()
    nw.add_signal_handler(loop)
    await nw.publish(nats_subject="model_ready", payload_df=encoded_nulog_json)
    logging.info(
        "Published to model_ready Nats subject that new Nulog model is ready to be used for inferencing."
    )
    await nw.nc.close()


if __name__ == "__main__":
    try:
        minio_client = boto3.resource(
            "s3",
            endpoint_url=MINIO_SERVER_URL,
            aws_access_key_id=MINIO_ACCESS_KEY,
            aws_secret_access_key=MINIO_SECRET_KEY,
            config=Config(signature_version="s3v4"),
        )

        logging.info("Connected to Minio client")
        minio_client.meta.client.download_file(
            "training-logs", "windows.tar.gz", "windows.tar.gz"
        )
        shutil.unpack_archive("windows.tar.gz", format="gztar")

        windows_folder_path = "windows/"
        bucket = minio_client.Bucket("nulog-models")
        exists = True
        try:
            minio_client.meta.client.head_bucket(Bucket="nulog-models")
        except botocore.exceptions.ClientError as e:
            # If a client error is thrown, then check that it was a 404 error.
            # If it was a 404 error, then the bucket does not exist.
            error_code = e.response["Error"]["Code"]
            if error_code == "404":
                exists = False
        if exists:
            logging.info("nulog-models bucket exists")
        else:
            logging.info("nulog-models bucket does not exist so creating it now")
            minio_client.create_bucket(Bucket="nulog-models")
        logging.info("About to train model")
        train_nulog_model(minio_client, windows_folder_path)
        logging.info("Model completed training")
        loop = asyncio.get_event_loop()
        send_signal_inference_coroutine = send_signal_to_inference(loop)
        loop.run_until_complete(send_signal_inference_coroutine)
        loop.close()
    except Exception as e:
        logging.info(
            "Cannot train Nulog model right now. Received exception {}".format(e)
        )
