# Standard Library
import asyncio
import json
import logging
import math
import os
import time
from collections import deque

# Third Party
import numpy as np
import pandas as pd
from drain3.file_persistence import FilePersistence
from drain3.template_miner import TemplateMiner
from nats_wrapper import NatsWrapper

pd.set_option("mode.chained_assignment", None)

# Third Party
from elasticsearch import AsyncElasticsearch
from elasticsearch.helpers import BulkIndexError, async_streaming_bulk

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")
persistence = FilePersistence("drain3_state.bin")
template_miner = TemplateMiner(persistence)
ES_ENDPOINT = os.environ["ES_ENDPOINT"]

num_no_templates_change_tracking_queue = deque([], 50)
num_templates_changed_tracking_queue = deque([], 50)
num_clusters_created_tracking_queue = deque([], 50)
num_total_clusters_tracking_queue = deque([], 50)


async def consume_logs(nw, loop, incoming_logs_to_train_queue, logs_to_update_es):
    if not nw.nc.is_connected:
        await nw.connect(loop)
        nw.add_signal_handler(loop)

    async def subscribe_handler(msg):
        subject = msg.subject
        reply = msg.reply
        payload_data = msg.data.decode()
        payload_data = msg.data.decode()
        await incoming_logs_to_train_queue.put(
            pd.read_json(payload_data, dtype={"_id": object})
        )

    async def anomalies_subscription_handler(msg):
        anomalies_data = msg.data.decode()
        logging.info("got anomaly payload")
        await logs_to_update_es.put(pd.read_json(anomalies_data, dtype={"_id": object}))

    await nw.subscribe(
        nats_subject="preprocessed_logs",
        payload_queue=None,
        subscribe_handler=subscribe_handler,
    )
    await nw.subscribe(
        nats_subject="preprocessed_logs_control_plane",
        payload_queue=None,
        subscribe_handler=subscribe_handler,
    )
    await nw.subscribe(
        nats_subject="anomalies",
        payload_queue=None,
        subscribe_handler=anomalies_subscription_handler,
    )


async def train_and_inference(nw, incoming_logs_to_train_queue, fail_keywords_str):
    global num_no_templates_change_tracking_queue, num_templates_changed_tracking_queue, num_clusters_created_tracking_queue
    while True:
        payload_data_df = await incoming_logs_to_train_queue.get()

        if not nw.nc.is_connected:
            await nw.connect(loop)
            nw.add_signal_handler(loop)

        inferencing_results = []

        for index, row in payload_data_df.iterrows():
            log_message = row["masked_log"]
            if log_message:
                result = template_miner.add_log_message(log_message)
                d = {
                    "_id": row["_id"],
                    "update_type": result["change_type"],
                    "template_or_change_type": result
                    if result == "cluster_created"
                    else str(result["cluster_id"]),
                    "matched_template": result["template_mined"],
                    "drain_matched_template_id": result["cluster_id"],
                    "drain_matched_template_support": result["cluster_size"],
                }
                inferencing_results.append(d)

        df = pd.DataFrame(inferencing_results)

        update_counts = df["update_type"].value_counts().to_dict()

        if "cluster_created" in update_counts:
            num_clusters_created_tracking_queue.appendleft(
                update_counts["cluster_created"]
            )

        if "cluster_template_changed" in update_counts:
            num_templates_changed_tracking_queue.appendleft(
                update_counts["cluster_template_changed"]
            )

        if "none" in update_counts:
            num_no_templates_change_tracking_queue.appendleft(update_counts["none"])

        # df['consecutive'] = df.template_or_change_type.groupby((df.template_or_change_type != df.template_or_change_type.shift()).cumsum()).transform('size')
        df["drain_prediction"] = 0
        if not fail_keywords_str:
            fail_keywords_str = "a^"
        df.loc[
            (df["drain_matched_template_support"] <= 10)
            & (df["drain_matched_template_support"] != 10)
            | (df["matched_template"].str.contains(fail_keywords_str, regex=True)),
            "drain_prediction",
        ] = 1
        prediction_payload = (
            df[
                [
                    "_id",
                    "drain_prediction",
                    "drain_matched_template_id",
                    "drain_matched_template_support",
                ]
            ]
            .to_json()
            .encode()
        )
        await nw.publish("anomalies", prediction_payload)


async def update_es_logs(queue):
    es = AsyncElasticsearch(
        [ES_ENDPOINT],
        port=9200,
        http_auth=("admin", "admin"),
        http_compress=True,
        max_retries=5,
        retry_on_timeout=True,
        timeout=10,
        use_ssl=True,
        verify_certs=False,
    )

    async def doc_generator_anomaly(df):
        for index, document in df.iterrows():
            doc_dict = document.to_dict()
            yield doc_dict

    async def doc_generator(df):
        for index, document in df.iterrows():
            doc_dict = document.to_dict()
            doc_dict["doc"] = {}
            doc_dict["doc"]["drain_matched_template_id"] = doc_dict[
                "drain_matched_template_id"
            ]
            doc_dict["doc"]["drain_matched_template_support"] = doc_dict[
                "drain_matched_template_support"
            ]
            del doc_dict["drain_matched_template_id"]
            del doc_dict["drain_matched_template_support"]
            yield doc_dict

    while True:
        df = await queue.get()
        df["_op_type"] = "update"
        df["_index"] = "logs"

        # update anomaly_predicted_count and anomaly_level for anomalous logs
        anomaly_df = df[df["drain_prediction"] == 1]
        if len(anomaly_df) == 0:
            logging.info("No anomalies in this payload")
        else:
            script = (
                "ctx._source.anomaly_predicted_count += 1; ctx._source.drain_anomaly = true; ctx._source.anomaly_level = "
                "ctx._source.anomaly_predicted_count == 1 ? 'Suspicious' : ctx._source.anomaly_predicted_count == 2 ? "
                "'Anomaly' : 'Normal';"
            )
            anomaly_df["script"] = script
            try:
                async for ok, result in async_streaming_bulk(
                    es,
                    doc_generator_anomaly(
                        anomaly_df[["_id", "_op_type", "_index", "script"]]
                    ),
                ):
                    action, result = result.popitem()
                    if not ok:
                        logging.error("failed to %s document %s" % ())
            except BulkIndexError as exception:
                logging.error("Failed to index data")
                logging.error(exception)
            logging.info("Updated {} anomalies in ES".format(len(anomaly_df)))

        try:
            # update normal logs in ES
            async for ok, result in async_streaming_bulk(
                es,
                doc_generator(
                    df[
                        [
                            "_id",
                            "_op_type",
                            "_index",
                            "drain_matched_template_id",
                            "drain_matched_template_support",
                        ]
                    ]
                ),
            ):
                action, result = result.popitem()
                if not ok:
                    logging.error("failed to %s document %s" % ())
        except BulkIndexError as exception:
            logging.error("Failed to index data")
            logging.error(exception)
        logging.info("Updated {} logs in ES".format(len(df)))


async def training_signal_check(nw):
    def weighted_avg_and_std(values, weights):
        average = np.average(values, weights=weights)
        # Fast and numerically precise:
        variance = np.average((values - average) ** 2, weights=weights)
        return average, math.sqrt(variance)

    iteration = 0
    train_on_next_chance = True
    stable = False
    training_start_ts_ns = time.time_ns()
    training_end_ts_ns = -1.0
    very_first_ts_ns = training_start_ts_ns
    previous_weighted_vol = -1.0

    normal_periods = []

    while True:
        await asyncio.sleep(20)
        iteration += 1
        num_total_clusters_tracking_queue.appendleft(len(template_miner.drain.clusters))
        logging.info(
            "training_signal_check: num_total_clusters_tracking_queue {}".format(
                num_total_clusters_tracking_queue
            )
        )
        num_clusters = np.array(num_total_clusters_tracking_queue)
        vol = np.std(num_clusters) / np.mean(num_clusters[:10])
        time_steps = num_clusters.shape[0]
        weights = np.flip(np.true_divide(np.arange(1, time_steps + 1), time_steps))
        weighted_mean, weighted_std = weighted_avg_and_std(num_clusters, weights)
        weighted_vol = weighted_std / np.mean(num_clusters[:10])
        logging.info(
            "ITERATION {}: vol= {} weighted_vol= {} normal_periods={}".format(
                iteration, vol, weighted_vol, str(normal_periods)
            )
        )

        if len(num_total_clusters_tracking_queue) > 3:
            if weighted_vol >= 0.199:
                train_on_next_chance = True

            if (
                weighted_vol < 0.199
                and training_start_ts_ns != very_first_ts_ns
                and train_on_next_chance
            ):
                training_start_ts_ns = time.time_ns()

            if weighted_vol > 0.155 and not train_on_next_chance and stable:
                training_end_ts_ns = time.time_ns()
                normal_periods.append(
                    {"start_ts": training_start_ts_ns, "end_ts": training_end_ts_ns}
                )
                stable = False
                training_start_ts_ns = -1.0

            if weighted_vol <= 0.15 and train_on_next_chance:
                logging.info("SENDING TRAIN SIGNAL on iteration {}".format(iteration))
                if training_start_ts_ns != -1.0:
                    training_end_ts_ns = time.time_ns()
                    normal_periods.append(
                        {"start_ts": training_start_ts_ns, "end_ts": training_end_ts_ns}
                    )
                # TODO send NATS signal
                train_payload = {
                    "model_to_train": "nulog",
                    "time_intervals": normal_periods,
                }
                await nw.publish("train", json.dumps(train_payload).encode())
                train_on_next_chance = False
                stable = True

                training_start_ts_ns = time.time_ns()

        previous_weighted_vol = weighted_vol

        logging.info([t.__str__() for t in template_miner.drain.clusters])


if __name__ == "__main__":
    fail_keywords_str = ""
    for fail_keyword in os.environ["FAIL_KEYWORDS"].split(","):
        if not fail_keyword:
            continue
        if len(fail_keywords_str) > 0:
            fail_keywords_str += "|({})".format(fail_keyword)
        else:
            fail_keywords_str += "({})".format(fail_keyword)
    logging.info("fail_keywords_str = {}".format(fail_keywords_str))

    nw = NatsWrapper()

    loop = asyncio.get_event_loop()
    incoming_logs_to_train_queue = asyncio.Queue(loop=loop)
    model_to_save_queue = asyncio.Queue(loop=loop)
    logs_to_update_in_elasticsearch = asyncio.Queue(loop=loop)

    preprocessed_logs_consumer_coroutine = consume_logs(
        nw, loop, incoming_logs_to_train_queue, logs_to_update_in_elasticsearch
    )
    train_coroutine = train_and_inference(
        nw, incoming_logs_to_train_queue, fail_keywords_str
    )
    update_es_coroutine = update_es_logs(logs_to_update_in_elasticsearch)
    training_signal_coroutine = training_signal_check(nw)

    loop.run_until_complete(
        asyncio.gather(
            preprocessed_logs_consumer_coroutine,
            train_coroutine,
            update_es_coroutine,
            training_signal_coroutine,
        )
    )
    try:
        loop.run_forever()
    finally:
        loop.close()
