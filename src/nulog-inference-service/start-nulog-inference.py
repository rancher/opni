# Standard Library
import asyncio
import logging
import os
import signal
import time

# Third Party
import pandas as pd
from elasticsearch import AsyncElasticsearch
from elasticsearch.helpers import async_streaming_bulk
from nats.aio.client import Client as NATS
from NulogServer import NulogServer

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)


async def consume_logs(loop, logs_queue):
    """
    coroutine to consume logs from NATS and put messages to the logs_queue
    """
    nc = NATS()
    NATS_SERVER_URL = os.environ["NATS_SERVER_URL"]

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
        logging.error(e)

    logging.info(f"Connected to NATS at {nc.connected_url.netloc}...")

    def signal_handler():
        if nc.is_closed:
            return
        logging.warning("Disconnecting...")
        loop.create_task(nc.close())

    for sig in ("SIGINT", "SIGTERM"):
        loop.add_signal_handler(getattr(signal, sig), signal_handler)

    async def subscribe_handler(msg):
        subject = msg.subject
        reply = msg.reply
        payload_data = msg.data.decode()
        await logs_queue.put(payload_data)

    await nc.subscribe("preprocessed_logs", "", subscribe_handler)


async def infer_logs(logs_queue):
    """
    coroutine to get payload from logs_queue, call inference rest API and put predictions to elasticsearch.
    """

    ES_ENDPOINT = os.environ["ES_ENDPOINT"]
    es = AsyncElasticsearch([ES_ENDPOINT], port=9200, http_compress=True)

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
        df = pd.read_json(payload, dtype={"_id": object})
        masked_log = list(df["masked_log"])
        predictions = nulog_predictor.predict(masked_log)

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

        # TODO: add these extra metadata in correct format
        #     df['script'] = df['script'] + "ctx._source.nulog_confidence = " + df['nulog_confidence'].map(str) + ";"

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


def start_inference_controller():
    """
    entry of inference controller.
    """
    loop = asyncio.get_event_loop()
    logs_queue = asyncio.Queue(loop=loop)

    consumer_coroutine = consume_logs(loop, logs_queue)
    inference_coroutine = infer_logs(logs_queue)

    loop.run_until_complete(asyncio.gather(inference_coroutine, consumer_coroutine))
    try:
        loop.run_forever()
    finally:
        loop.close()


if __name__ == "__main__":
    start_inference_controller()
