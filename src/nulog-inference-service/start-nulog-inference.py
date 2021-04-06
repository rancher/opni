# Standard Library
import asyncio
import gc
import json
import logging
import os
import time

# Third Party
import pandas as pd
from elasticsearch import AsyncElasticsearch
from elasticsearch.helpers import async_streaming_bulk
from nats_wrapper import NatsWrapper
from NulogServer import NulogServer

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)


async def consume_logs(nw, loop, logs_queue):
    """
    coroutine to consume logs from NATS and put messages to the logs_queue
    """
    await nw.connect(loop)
    nw.add_signal_handler(loop)
    await nw.subscribe(nats_subject="preprocessed_logs", payload_queue=logs_queue)
    await nw.subscribe(nats_subject="model_ready", payload_queue=logs_queue)


async def infer_logs(logs_queue):
    """
    coroutine to get payload from logs_queue, call inference rest API and put predictions to elasticsearch.
    """

    ES_ENDPOINT = os.environ["ES_ENDPOINT"]
    es = AsyncElasticsearch(
        [ES_ENDPOINT],
        port=9200,
        http_compress=True,
        http_auth=("admin", "admin"),
        verify_certs=False,
        use_ssl=True,
    )

    nulog_predictor = NulogServer()

    async def doc_generator(df):
        for index, document in df.iterrows():
            doc_dict = document.to_dict()
            yield doc_dict

    threshold = 0.8
    script = 'ctx._source.anomaly_level = ctx._source.anomaly_predicted_count == 0 ? "Normal" : ctx._source.anomaly_predicted_count == 1 ? "Suspicious" : "Anomaly";'

    while True:
        payload = await logs_queue.get()
        if payload is None:
            continue

        start_time = time.time()
        decoded_payload = json.loads(payload)
        ## TODO: testing decode with pd.read_json first to reduce unnecessary decode.
        ## logic: df = pd.read_json(payload, dtype={"_id": object})
        ## if "bucket" in df: reload nulog model
        if "bucket" in decoded_payload and decoded_payload["bucket"] == "nulog-models":
            logging.info(
                "Just received signal to download a new Nulog model files from Minio."
            )
            nulog_predictor.download_from_minio(decoded_payload)
            nulog_predictor.load()
            continue

        df = pd.read_json(payload, dtype={"_id": object})
        masked_log = list(df["masked_log"])
        logging.info("inferencing payload.")
        predictions = nulog_predictor.predict(masked_log)
        if predictions is None:
            continue

        df["nulog_confidence"] = predictions
        df["predictions"] = [1 if p < threshold else 0 for p in predictions]
        # filter out df to only include abnormal predictions
        df = df[df["predictions"] > 0]
        if len(df) == 0:
            logging.info(
                "No anomalies in this payload of {} logs".format(len(masked_log))
            )
            continue

        df["_op_type"] = "update"
        df["_index"] = "logs"
        df["script"] = (
            "ctx._source.anomaly_predicted_count += 1; ctx._source.nulog_anomaly = true;"
            + script
        )

        df.rename(columns={"log_id": "_id"}, inplace=True)
        df["script"] = (
            df["script"]
            + "ctx._source.nulog_confidence = "
            + df["nulog_confidence"].map(str)
            + ";"
        )

        try:
            async for ok, result in async_streaming_bulk(
                es, doc_generator(df[["_id", "_op_type", "_index", "script"]])
            ):
                action, result = result.popitem()
                if not ok:
                    logging.error("failed to %s document %s" % ())
        except Exception as e:
            logging.error(e)
        finally:
            logging.info(
                "Updated {} anomalies from {} logs to ES".format(
                    len(df), len(masked_log)
                )
            )
            logging.info("Updated in {} seconds".format(time.time() - start_time))

        del df
        del masked_log

        del decoded_payload
        gc.collect()


def start_inference_controller():
    """
    entry of inference controller.
    """
    loop = asyncio.get_event_loop()
    logs_queue = asyncio.Queue(loop=loop)

    nw = NatsWrapper()
    consumer_coroutine = consume_logs(nw, loop, logs_queue)
    inference_coroutine = infer_logs(logs_queue)

    loop.run_until_complete(asyncio.gather(inference_coroutine, consumer_coroutine))
    try:
        loop.run_forever()
    finally:
        loop.close()


if __name__ == "__main__":
    start_inference_controller()
