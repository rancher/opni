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
from elasticsearch.helpers import async_bulk
from nats_wrapper import NatsWrapper
from NulogServer import NulogServer

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)
THRESHOLD = float(os.getenv("MODEL_THRESHOLD", 0.7))
ES_ENDPOINT = os.environ["ES_ENDPOINT"]
IS_CONTROL_PLANE_SERVICE = bool(os.getenv("IS_CONTROL_PLANE_SERVICE", False))


async def consume_logs(nw, logs_queue):
    """
    coroutine to consume logs from NATS and put messages to the logs_queue
    """
    while True:
        if nw.first_run_or_got_disconnected_or_error:
            logging.info("Need to (re)connect to NATS")
            nw.re_init()
            await nw.connect()
            if IS_CONTROL_PLANE_SERVICE:
                await nw.subscribe(
                    nats_subject="preprocessed_logs_control_plane",
                    payload_queue=logs_queue,
                    nats_queue="workers",
                )
            else:
                await nw.subscribe(
                    nats_subject="preprocessed_logs", payload_queue=logs_queue
                )
                await nw.subscribe(nats_subject="model_ready", payload_queue=logs_queue)
            nw.first_run_or_got_disconnected_or_error = False
        await asyncio.sleep(1)


async def infer_logs(logs_queue):
    """
    coroutine to get payload from logs_queue, call inference rest API and put predictions to elasticsearch.
    """
    es = AsyncElasticsearch(
        [ES_ENDPOINT],
        port=9200,
        http_compress=True,
        http_auth=("admin", "admin"),
        verify_certs=False,
        use_ssl=True,
    )

    nulog_predictor = NulogServer()
    if IS_CONTROL_PLANE_SERVICE:
        nulog_predictor.load(save_path="control-plane-output/")
    else:
        nulog_predictor.download_from_minio()
        nulog_predictor.load()

    async def doc_generator(df):
        for index, document in df.iterrows():
            doc_dict = document.to_dict()
            yield doc_dict

    if IS_CONTROL_PLANE_SERVICE:
        script_source = 'ctx._source.anomaly_level = ctx._source.anomaly_predicted_count == 0 ? "Normal" : "Anomaly";'
    else:
        script_source = 'ctx._source.anomaly_level = ctx._source.anomaly_predicted_count == 0 ? "Normal" : ctx._source.anomaly_predicted_count == 1 ? "Suspicious" : "Anomaly";'
    script_source += "ctx._source.nulog_confidence = params['nulog_score'];"
    script_for_anomaly = (
        "ctx._source.anomaly_predicted_count += 1; ctx._source.nulog_anomaly = true;"
    )

    max_payload_size = 128 if IS_CONTROL_PLANE_SERVICE else 512
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
            nulog_predictor.download_from_minio(decoded_payload)
            nulog_predictor.load()
            continue

        df_payload = pd.read_json(payload, dtype={"_id": object})
        for i in range(0, len(df_payload), max_payload_size):
            df = df_payload[i : min(i + max_payload_size, len(df_payload))]
            masked_log = list(df["masked_log"])
            predictions = nulog_predictor.predict(masked_log)
            if predictions is None:
                logging.warning("fail to make predictions.")
                continue

            df["nulog_confidence"] = predictions
            df["predictions"] = [1 if p < THRESHOLD else 0 for p in predictions]
            # filter out df to only include abnormal predictions

            df["_op_type"] = "update"
            df["_index"] = "logs"
            df.rename(columns={"log_id": "_id"}, inplace=True)
            df["script"] = [
                {
                    "source": (script_for_anomaly + script_source)
                    if nulog_score < THRESHOLD
                    else script_source,
                    "lang": "painless",
                    "params": {"nulog_score": nulog_score},
                }
                for nulog_score in df["nulog_confidence"]
            ]
            try:
                await async_bulk(
                    es, doc_generator(df[["_id", "_op_type", "_index", "script"]])
                )
                logging.info(
                    "Updated {} anomalies from {} logs to ES in {} seconds".format(
                        len(df[df["predictions"] > 0]),
                        len(masked_log),
                        time.time() - start_time,
                    )
                )
            except Exception as e:
                logging.error(e)

            del df
            del masked_log

        del decoded_payload
        del df_payload
        gc.collect()


if __name__ == "__main__":
    loop = asyncio.get_event_loop()
    logs_queue = asyncio.Queue(loop=loop)

    nw = NatsWrapper(loop)
    consumer_coroutine = consume_logs(nw, logs_queue)
    inference_coroutine = infer_logs(logs_queue)

    loop.run_until_complete(asyncio.gather(inference_coroutine, consumer_coroutine))
    try:
        loop.run_forever()
    finally:
        loop.close()
