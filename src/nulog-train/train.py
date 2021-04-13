# Standard Library
import asyncio
import json
import logging
import os
import shutil
import signal

# Third Party
import boto3
import botocore
from botocore.client import Config
from nats.aio.client import Client as NATS
from NuLogParser import LogParser

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")
MINIO_ACCESS_KEY = os.environ["MINIO_ACCESS_KEY"]
MINIO_SECRET_KEY = os.environ["MINIO_SECRET_KEY"]
NATS_SERVER_URL = os.environ["NATS_SERVER_URL"]
minio_client = boto3.resource(
    "s3",
    endpoint_url="http://minio.default.svc.cluster.local:9000",
    aws_access_key_id=MINIO_ACCESS_KEY,
    aws_secret_access_key=MINIO_SECRET_KEY,
    config=Config(signature_version="s3v4"),
)
logging.info("Connected to Minio client")
minio_client.meta.client.download_file(
    "training-logs", "windows.tar.gz", "windows.tar.gz"
)
logging.info("Downloaded the windows.tar.gz file")
shutil.unpack_archive("windows.tar.gz", format="gztar")
logging.info("Extracted windows directory from windows.tar.gz file")

WINDOWS_FOLDER_PATH = "windows/"


def train_nulog_model(minio_client):
    nr_epochs = 2
    num_samples = 0
    parser = LogParser()
    texts = parser.load_data(WINDOWS_FOLDER_PATH)
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


async def run(loop, nc):
    async def error_cb(e):
        logging.warning("Error: {}".format(str(e)))

    async def closed_cb():
        logging.warning("Closed connection to NATS")
        await asyncio.sleep(0.1, loop=loop)
        loop.stop()

    async def on_disconnect():
        logging.warning("Disconnected from NATS")

    async def reconnected_cb():
        logging.warning(
            "Reconnected to NATS at nats://{}".format(nats.connected_url.netloc)
        )

    options = {
        "loop": loop,
        "error_cb": error_cb,
        "closed_cb": closed_cb,
        "reconnected_cb": reconnected_cb,
        "disconnected_cb": on_disconnect,
        "servers": [NATS_SERVER_URL],
    }

    try:
        await nc.connect(**options)
    except Exception as e:
        logging.error(str(e))

    logging.info(f"Connected to NATS at {nc.connected_url.netloc}...")

    def signal_handler():
        if nc.is_closed:
            return
        logging.warning("Disconnecting...")
        loop.create_task(nc.close())

    for sig in ("SIGINT", "SIGTERM"):
        loop.add_signal_handler(getattr(signal, sig), signal_handler)

    nulog_payload = {
        "bucket": "nulog-models",
        "bucket_files": {
            "model_file": "nulog_model_latest.pt",
            "vocab_file": "vocab.txt",
        },
    }
    encoded_nulog_json = json.dumps(nulog_payload).encode()
    await nc.publish("model_ready", encoded_nulog_json)
    logging.info(
        "Published to model_ready Nats subject that the latest Nulog has been uploaded onto Minio."
    )
    await nc.close()


if __name__ == "__main__":
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
    train_nulog_model(minio_client)
    logging.info("Model completed training")
    nc = NATS()
    loop = asyncio.get_event_loop()
    loop.run_until_complete(run(loop, nc))
    loop.close()
