# Standard Library
import asyncio
import logging
import os
from datetime import datetime

# Third Party
import pandas as pd
from elasticsearch import AsyncElasticsearch
from elasticsearch.helpers import async_streaming_bulk
from masker import LogMasker
from nats_wrapper import NatsWrapper

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")

ES_ENDPOINT = os.environ["ES_ENDPOINT"]

es = AsyncElasticsearch(
    [ES_ENDPOINT],
    port=9200,
    http_auth=("admin", "admin"),
    http_compress=True,
    verify_certs=False,
    use_ssl=True,
    timeout=10,
    max_retries=5,
    retry_on_timeout=True,
)


async def doc_generator(df):
    df["_index"] = "logs"
    df["anomaly_predicted_count"] = 0
    df["nulog_anomaly"] = False
    df["drain_anomaly"] = False
    df["nulog_confidence"] = -1.0
    df["drain_matched_template_id"] = -1.0
    df["drain_matched_template_support"] = -1.0
    df["anomaly_level"] = "Normal"
    for index, document in df.iterrows():
        doc_dict = document.to_dict()
        yield doc_dict


async def consume_logs(nw, mask_logs_queue):
    async def subscribe_handler(msg):
        payload_data = msg.data.decode()
        await mask_logs_queue.put(pd.read_json(payload_data, dtype={"_id": object}))

    while True:
        if nw.first_run_or_got_disconnected_or_error:
            logging.info("Need to (re)connect to NATS")
            nw.re_init()
            await nw.connect()
            await nw.subscribe(
                nats_subject="raw_logs",
                nats_queue="workers",
                payload_queue=mask_logs_queue,
                subscribe_handler=subscribe_handler,
            )
            nw.first_run_or_got_disconnected_or_error = False
        await asyncio.sleep(1)


async def mask_logs(nw, queue):
    masker = LogMasker()
    while True:
        payload_data_df = await queue.get()
        payload_data_df["log"] = payload_data_df["log"].str.strip()
        masked_logs = []
        for index, row in payload_data_df.iterrows():
            masked_logs.append(masker.mask(row["log"]))
        payload_data_df["masked_log"] = masked_logs
        # impute NaT with time column if available else use current time
        payload_data_df["time_operation"] = pd.to_datetime(
            payload_data_df["time"], errors="coerce", utc=True
        )
        for index, i in payload_data_df.loc[
            payload_data_df["time_operation"].isna()
        ].iterrows():
            payload_data_df.loc[index, "time_operation"] = datetime.utcfromtimestamp(
                int(i["time"]) / 1000.0
            )
        payload_data_df["time"] = payload_data_df["time_operation"]
        payload_data_df.drop(columns=["time_operation"], inplace=True)
        if "timestamp" in payload_data_df.columns:
            payload_data_df.loc[
                (payload_data_df.timestamp.dt.year == 1970), "timestamp"
            ] = pd.NaT
            payload_data_df["timestamp"].fillna(
                pd.to_datetime(
                    payload_data_df.time, unit="ms", errors="ignore", utc=True
                ),
                inplace=True,
            )
            payload_data_df["timestamp"].fillna(pd.to_datetime("now", utc=True))
            payload_data_df["timestamp"] = pd.to_datetime(
                payload_data_df["timestamp"], utc=True
            )
        else:
            payload_data_df["timestamp"] = payload_data_df["time"]

        # drop redundant field in control plane logs
        payload_data_df.drop(["t.$date"], axis=1, errors="ignore", inplace=True)
        payload_data_df["is_control_plane_log"] = False
        payload_data_df["kubernetes_component"] = ""
        if "filename" in payload_data_df.columns:
            payload_data_df["is_control_plane_log"] = payload_data_df[
                "filename"
            ].str.contains(
                "rke/log/etcd|rke/log/kubelet|/rke/log/kube-apiserver|rke/log/kube-controller-manager|rke/log/kube-proxy|rke/log/kube-scheduler"
            )
            payload_data_df["kubernetes_component"] = payload_data_df["filename"].apply(
                lambda x: os.path.basename(x)
            )
            payload_data_df["kubernetes_component"] = (
                payload_data_df["kubernetes_component"].str.split("_").str[0]
            )
        is_control_log = payload_data_df["is_control_plane_log"] == True
        control_plane_logs_df = payload_data_df[is_control_log]
        app_logs_df = payload_data_df[~is_control_log]

        if not nw.nc.is_connected:
            await nw.connect()

        if len(app_logs_df) > 0:
            await nw.publish("preprocessed_logs", app_logs_df.to_json().encode())

        if len(control_plane_logs_df) > 0:
            await nw.publish(
                "preprocessed_logs_control_plane",
                control_plane_logs_df.to_json().encode(),
            )

        async for ok, result in async_streaming_bulk(
            es, doc_generator(payload_data_df)
        ):
            action, result = result.popitem()
            if not ok:
                logging.error("failed to %s document %s" % ())


if __name__ == "__main__":
    loop = asyncio.get_event_loop()
    mask_logs_queue = asyncio.Queue(loop=loop)
    nw = NatsWrapper(loop)
    nats_consumer_coroutine = consume_logs(nw, mask_logs_queue)
    mask_logs_coroutine = mask_logs(nw, mask_logs_queue)
    loop.run_until_complete(
        asyncio.gather(nats_consumer_coroutine, mask_logs_coroutine)
    )
    try:
        loop.run_forever()
    finally:
        loop.close()
