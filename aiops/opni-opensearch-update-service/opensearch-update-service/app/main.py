# Standard Library
import asyncio
import json
import logging
import os
import sys
import time

# Third Party
from opni_proto.log_anomaly_payload_pb import PayloadList
import pandas as pd
from elasticsearch import AsyncElasticsearch, TransportError
from elasticsearch.exceptions import ConnectionTimeout
from elasticsearch.helpers import BulkIndexError, async_streaming_bulk

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")

pretrained_model_script_source = 'ctx._source.anomaly_level = ctx._source.anomaly_predicted_count != 0 ? "Anomaly" : "Normal";'
workload_script_source = 'ctx._source.anomaly_level = ctx._source.anomaly_predicted_count == 0 ? "Normal" : ctx._source.anomaly_predicted_count == 1 ? "Suspicious" : "Anomaly";'
pretrained_model_script_source += "ctx._source.opnilog_confidence = params['opnilog_score'];"
workload_script_source += "ctx._source.opnilog_confidence = params['opnilog_score'];"
script_for_anomaly = (
    "ctx._source.anomaly_predicted_count += 1; ctx._source.opnilog_anomaly = true;"
)

async def doc_generator(df, index, op_type="update"):
    main_doc_keywords = {"_op_type", "_index", "_id", "doc"}
    df["_op_type"] = op_type
    df["_index"] = index
    df.rename(columns={"log_id": "_id"}, inplace=True)
    for index, document in df.iterrows():
        doc_dict = document.to_dict()
        doc_dict["doc"] = {}
        doc_dict_keys = list(doc_dict.keys())
        for k in doc_dict_keys:
            if not k in main_doc_keywords:
                doc_dict["doc"][k] = doc_dict[k]
                del doc_dict[k]
        yield doc_dict

async def setup_es_connection():
    ES_ENDPOINT = os.environ["ES_ENDPOINT"]
    ES_USERNAME = os.environ["ES_USERNAME"]
    ES_PASSWORD = os.environ["ES_PASSWORD"]
    # This function will be setting up the Opensearch connection.
    logging.info("Setting up AsyncElasticsearch")
    return AsyncElasticsearch(
        [ES_ENDPOINT],
        port=9200,
        http_auth=(ES_USERNAME, ES_PASSWORD),
        http_compress=True,
        max_retries=10,
        retry_on_status={100, 400, 503},
        retry_on_timeout=True,
        timeout=20,
        use_ssl=True,
        verify_certs=False,
        sniff_on_start=False,
        # refresh nodes after a node fails to respond
        sniff_on_connection_fail=True,
        # and also every 60 seconds
        sniffer_timeout=60,
        sniff_timeout=10,
    )


async def consume_logs(nw, inferenced_logs_queue, template_logs_queue):
    async def subscribe_handler(msg):
        data = msg.data
        logs_df = pd.DataFrame(PayloadList().parse(data).items)
        await inferenced_logs_queue.put(logs_df)

    async def template_subscribe_handler(msg):
        data = msg.data
        logs_df = pd.DataFrame(PayloadList().parse(data).items)
        await template_logs_queue.put(logs_df)

    await nw.subscribe(
        nats_subject="inferenced_logs",
        nats_queue="workers",
        payload_queue=inferenced_logs_queue,
        subscribe_handler=subscribe_handler,
    )

    await nw.subscribe(
        nats_subject="templates_index",
        nats_queue="workers",
        payload_queue=template_logs_queue,
        subscribe_handler=template_subscribe_handler,
    )

async def receive_template_data(queue):
    es = await setup_es_connection()
    while True:
        df = await queue.get()
        await update_template_data(es, df)


async def receive_logs(queue):
    es = await setup_es_connection()
    while True:
        df = await queue.get()
        await update_logs(es, df)

async def update_template_data(es, df):
    try:
        async for ok, result in async_streaming_bulk(
                es,
                doc_generator(df[["_id", "log", "template_matched", "template_cluster_id"]], "templates", "index"),
                max_retries=1,
                initial_backoff=1,
                request_timeout=5,
        ):
            action, result = result.popitem()
            if not ok:
                logging.error("failed to {} document {}".format())
    except (BulkIndexError, ConnectionTimeout, TimeoutError) as exception:
        logging.error(
            "Failed to index data. Re-adding to logs_to_update_in_elasticsearch queue"
        )
        logging.error(exception)
    except TransportError as exception:
        logging.info(f"Error in async_streaming_bulk {exception}")
        if exception.status_code == "N/A":
            logging.info("Elasticsearch connection error")
            es = await setup_es_connection()

async def update_logs(es, df):
    # This function will be updating Opensearch logs which were inferred on by the DRAIN model.
    model_keywords_dict = {"drain":  ["_id", "masked_log", "template_matched","template_cluster_id","inference_model", "anomaly_level"],
                          "opnilog":  ["_id", "masked_log", "anomaly_level", "template_matched","template_cluster_id","opnilog_confidence", "inference_model"]}
    anomaly_level_options = ["Normal", "Anomaly"]
    pretrained_model_logs_df = df.loc[(df["log_type"] != "workload")]
    for model_name in model_keywords_dict:
        model_df = pretrained_model_logs_df[pretrained_model_logs_df["inference_model"] == model_name]
        for anomaly_level in anomaly_level_options:
            anomaly_level_df = model_df[model_df["anomaly_level"] == anomaly_level]
            if len(anomaly_level_df) == 0:
                continue
            try:
                async for ok, result in async_streaming_bulk(
                        es,
                        doc_generator(
                            anomaly_level_df[model_keywords_dict[model_name]],
                            "logs",
                        ),
                        max_retries=1,
                        initial_backoff=1,
                        request_timeout=5,
                ):
                    action, result = result.popitem()
                    if not ok:
                        logging.error("failed to {} document {}".format())
            except (BulkIndexError, ConnectionTimeout, TimeoutError) as exception:
                logging.error(
                    "Failed to index data. Re-adding to logs_to_update_in_elasticsearch queue"
                )
                logging.error(exception)
            except TransportError as exception:
                logging.info(f"Error in async_streaming_bulk {exception}")
                if exception.status_code == "N/A":
                    logging.info("Elasticsearch connection error")
                    es = await setup_es_connection()

async def init_nats():
    from opni_nats import NatsWrapper
    logging.info("Attempting to connect to NATS")
    nw = NatsWrapper()
    await nw.connect()
    return nw


if __name__ == "__main__":
    loop = asyncio.get_event_loop()
    processed_logs_queue = asyncio.Queue(loop=loop)
    template_logs_queue = asyncio.Queue(loop=loop)
    init_nats_task = loop.create_task(init_nats())
    nw = loop.run_until_complete(init_nats_task)
    nats_consumer_coroutine = consume_logs(nw, processed_logs_queue, template_logs_queue)
    update_logs_coroutine = receive_logs(processed_logs_queue)
    update_templates_coroutine = receive_template_data(template_logs_queue)


    loop.run_until_complete(
        asyncio.gather(nats_consumer_coroutine, update_logs_coroutine, update_templates_coroutine)
    )
    try:
        loop.run_forever()
    finally:
        loop.close()
