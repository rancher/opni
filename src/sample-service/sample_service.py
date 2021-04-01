import asyncio
from nats.aio.client import Client as NATS
import logging
import os
import signal
import json

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")
NATS_SERVER_URL = os.environ["NATS_SERVER_URL"]

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
        logging.warning("Reconnected to NATS at nats://{}".format(nats.connected_url.netloc))

    options = {
        "loop": loop,
        "error_cb": error_cb,
        "closed_cb": closed_cb,
        "reconnected_cb": reconnected_cb,
        "disconnected_cb": on_disconnect,
        "servers": [NATS_SERVER_URL]
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

    for sig in ('SIGINT', 'SIGTERM'):
        loop.add_signal_handler(getattr(signal, sig), signal_handler)
    
    #payload = {"model_to_train": "nulog","time_intervals": [{"start_ts": 1617039390000000000, "end_ts": 1617041340000000000}, {"start_ts": 1617041400000000000, "end_ts": 1617041850000000000}]}
    payload = {"model_to_train": "nulog","time_intervals": [{"start_ts": 1617039360000000000, "end_ts": 1617039450000000000}, {"start_ts": 1617039510000000000, "end_ts": 1617039660000000000}]}
    second_payload = {"model_to_train": "nulog","time_intervals": [{"start_ts": 1617039930000000000, "end_ts": 1617040020000000000}]}
    encoded_payload_json = json.dumps(payload).encode()
    await nc.publish("train", encoded_payload_json)
    encoded_second_payload_json = json.dumps(second_payload).encode()
    await nc.publish("train", encoded_second_payload_json)
    logging.info("Published to model_ready Nats subject that the latest Fasttext and PCA model has been uploaded onto Minio.")
    await nc.close()

if __name__ == "__main__":
    nc = NATS()
    loop = asyncio.get_event_loop()
    loop.run_until_complete(run(loop, nc))
    loop.close()
