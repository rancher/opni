import boto3
from botocore.client import Config
import logging
import os
import subprocess
from subprocess import Popen
import shutil
import tarfile
import time
import pandas as pd

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")

# from prepare_training_logs import PrepareTrainingLogs
class PrepareTrainingLogs():
    def __init__(self, working_dir):
        self.WORKING_DIR = working_dir
        self.ES_DUMP_DIR = os.path.join(self.WORKING_DIR, "windows")
        self.ES_DUMP_DIR_ZIPPED = self.ES_DUMP_DIR + ".tar.gz"
        self.ES_DUMP_SAMPLE_LOGS_PATH = os.path.join(self.WORKING_DIR, "sample_logs.json")

        MINIO_ACCESS_KEY = os.environ["MINIO_ACCESS_KEY"]
        MINIO_SECRET_KEY = os.environ["MINIO_SECRET_KEY"]
        self.minio_client = boto3.resource('s3', endpoint_url='http://minio.default.svc.cluster.local:9000',
                                           aws_access_key_id=MINIO_ACCESS_KEY, aws_secret_access_key=MINIO_SECRET_KEY,
                                           config=Config(signature_version='s3v4'))
        self.es_dump_data_path = ""

    def save_window(self, window_start_time_ns, df):
        df[['time_nanoseconds', 'window_start_time_ns', 'masked_log']].to_json(
            os.path.join(self.ES_DUMP_DIR, str(window_start_time_ns) + ".json.gz"), orient='records', lines=True,
            compression='gzip')

    def disk_size(self):
        # Fetch size of disk
        logging.info("Fetching size of the disk")
        total, used, free = shutil.disk_usage("/")
        logging.info("Disk Total: %d GiB" % (total // (2 ** 30)))
        logging.info("Disk Used: %d GiB" % (used // (2 ** 30)))
        logging.info("Disk Free: %d GiB" % (free // (2 ** 30)))
        return free

    def run_esdump(self, chunked_commands):
        processes = set()
        max_processes = 200
        counter = 0
        
        for index,chunk in enumerate(chunked_commands):
            for query in chunk:
                processes.add(subprocess.Popen(query))
                counter += 1        
            for p in processes:
                if p.poll() is None:
                    p.wait()
            
            logging.info("completed chunk {}".format(index))

    def retrieve_sample_logs(self):
        # Get the first 10k logs
        logging.info("Retrieve sample logs from ES")
        es_dump_cmd = 'elasticdump --searchBody \'{"query": { "match_all": {} }, "_source": ["masked_log", "time_nanoseconds"]}\' --retryAttempts 10 --size=10000 --limit 10000 --input=http://elasticsearch-coordinating-only.default.svc.cluster.local:9200/logs --output=%s --type=data' % self.ES_DUMP_SAMPLE_LOGS_PATH
        subprocess.run(es_dump_cmd, shell=True)

        if os.path.exists(self.ES_DUMP_SAMPLE_LOGS_PATH):
            logging.info("Sampled downloaded successfully")
        else:
            logging.error("Sample failed to download")

    def calculate_training_logs_size(self, free):
        # Determine average size per log message
        sample_logs_bytes_size = os.path.getsize(self.ES_DUMP_SAMPLE_LOGS_PATH)
        num_lines = sum(1 for line in open(self.ES_DUMP_SAMPLE_LOGS_PATH))
        average_size_per_log_message = sample_logs_bytes_size / num_lines
        logging.info("\naverage size per log message = {} bytes".format(average_size_per_log_message))
        os.remove(self.ES_DUMP_SAMPLE_LOGS_PATH)
        # Determine number of logs to fetch for training
        num_logs_to_fetch = int((free * 0.8) / average_size_per_log_message)
        logging.info("Number of log messages to fetch = {}".format(num_logs_to_fetch))
        return num_logs_to_fetch

    def fetch_training_logs(self, num_logs_to_fetch, timestamps_list):
        # ESDump logs
        esdump_sample_command = ['elasticdump', '--searchBody', '{{"query": {{"range": {{"time_nanoseconds": {{"gte": {}, "lt": {}}}}}}}, "_source": ["masked_log", "time_nanoseconds", "window_start_time_ns", "_id"], "sort": [{{"time_nanoseconds": {{"order": "asc"}}}}]}}', '--retryAttempts', '100', '--fileSize=50mb','--size={}','--limit', '10000', '--input=http://elasticsearch-coordinating-only.default.svc.cluster.local:9200/logs', '--output={}', '--type=data']
        query_queue = []
        for idx, entry in enumerate(timestamps_list):
            current_command = esdump_sample_command[:]
            current_command[2] = current_command[2].format(entry["start_ts"], entry["end_ts"])
            current_command[6] = current_command[6].format(num_logs_to_fetch)
            current_command[10] = current_command[10].format(os.path.join(self.es_dump_data_path, "training_logs_{}.json".format(idx)))
            query_queue.append(current_command)
        chunked_commands = [query_queue[i:i+5] for i in range(0,len(query_queue),5)]
        self.run_esdump(chunked_commands)

    def create_windows(self):
        # For every json file, write/append each time window to own file
        for es_split_json_file in os.listdir(self.es_dump_data_path):
            if not '.json' in es_split_json_file:
                continue
            json_file_to_process = os.path.join(self.es_dump_data_path, es_split_json_file)
            df = pd.read_json(json_file_to_process, lines=True)
            df = pd.json_normalize(df['_source'])
            df.groupby(['window_start_time_ns']).apply(lambda x: self.save_window(x.name, x))
            # delete ESDumped file
            os.remove(json_file_to_process)
        shutil.rmtree(self.es_dump_data_path)

    def tar_windows_folder(self):
        # tar windows folder
        with tarfile.open(self.ES_DUMP_DIR_ZIPPED, "w:gz") as tar:
            tar.add(self.ES_DUMP_DIR, arcname=os.path.basename(self.ES_DUMP_DIR))
        shutil.rmtree(self.ES_DUMP_DIR)

    def upload_windows_tar_to_minio(self):
        # upload to minio
        if not self.minio_client.Bucket('training-logs').creation_date:
            self.minio_client.meta.client.create_bucket(Bucket='training-logs')
        self.minio_client.meta.client.upload_file(self.ES_DUMP_DIR_ZIPPED, 'training-logs', os.path.basename(self.ES_DUMP_DIR_ZIPPED))

    def run(self, timestamps_list):
        self.es_dump_data_path = os.path.join(self.WORKING_DIR, "esdump_data/")
        if not os.path.exists(self.es_dump_data_path):
            os.makedirs(self.es_dump_data_path)
        if not os.path.exists(self.ES_DUMP_DIR):
            os.makedirs(self.ES_DUMP_DIR)

        free = self.disk_size()
        self.retrieve_sample_logs()
        num_logs_to_fetch = self.calculate_training_logs_size(free)
        self.fetch_training_logs(num_logs_to_fetch, timestamps_list)
        self.create_windows()
        self.tar_windows_folder()
        self.upload_windows_tar_to_minio()
