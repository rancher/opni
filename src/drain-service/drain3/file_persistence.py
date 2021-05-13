"""
Description : This file implements the persist/restore from file
Author      : Moshik Hershcovitch
Author_email: moshikh@il.ibm.com
License     : MIT
"""

# Standard Library
import os
import pathlib
import boto3
import botocore
from botocore.client import Config
import logging

# Third Party
from drain3.persistence_handler import PersistenceHandler

MINIO_SERVER_URL = os.environ["MINIO_SERVER_URL"]
MINIO_ACCESS_KEY = os.environ["MINIO_ACCESS_KEY"]
MINIO_SECRET_KEY = os.environ["MINIO_SECRET_KEY"]

class FilePersistence(PersistenceHandler):
    def __init__(self, file_path):
        self.file_path = file_path
        self.connect_to_minio()

    def connect_to_minio(self):
        self.minio_client = None
        try:
            self.minio_client = boto3.resource(
                "s3",
                endpoint_url=MINIO_SERVER_URL,
                aws_access_key_id=MINIO_ACCESS_KEY,
                aws_secret_access_key=MINIO_SECRET_KEY,
                config=Config(signature_version="s3v4"),
            )
            logging.info("Connected to Minio client")
        except Exception as e:
            logging.error("Unable to connect to Minio client right now.")

        exists = True
        try:
            self.minio_client.meta.client.head_bucket(Bucket="drain-model")
        except botocore.exceptions.ClientError as e:
            # If a client error is thrown, then check that it was a 404 error.
            # If it was a 404 error, then the bucket does not exist.
            error_code = e.response["Error"]["Code"]
            if error_code == "404":
                exists = False
        if exists:
            logging.info("drain-model bucket exists")
        else:
            logging.info("drain-model bucket does not exist so creating it now")
            self.minio_client.create_bucket(Bucket="drain-model")

    def save_state(self, state):
        pathlib.Path(self.file_path).write_bytes(state)
        try:
            self.minio_client.meta.client.upload_file(self.file_path, "drain-model", self.file_path)
            logging.info("Saved DRAIN model into Minio")
        except Exception as e:
            logging.error("Unable to save DRAIN model into Minio")

    def load_state(self):
        try:
            self.minio_client.meta.client.download_file(
                "drain-model", self.file_path, self.file_path)
            logging.info("Downloaded DRAIN model file from Minio")
        except Exception as e:
            logging.error(
                "Cannot currently obtain DRAIN model file"
            )
        if not os.path.exists(self.file_path):
            return None

        return pathlib.Path(self.file_path).read_bytes()
