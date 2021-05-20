# Standard Library
import asyncio
import json
import logging
import os
import time
from datetime import datetime

# Third Party
import kubernetes.client
from elasticsearch import AsyncElasticsearch, exceptions
from elasticsearch.helpers import async_streaming_bulk
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from nats_wrapper import NatsWrapper
from prepare_training_logs import PrepareTrainingLogs

MINIO_SERVER_URL = os.environ["MINIO_SERVER_URL"]
MINIO_ACCESS_KEY = os.environ["MINIO_ACCESS_KEY"]
MINIO_SECRET_KEY = os.environ["MINIO_SECRET_KEY"]
NATS_SERVER_URL = os.environ["NATS_SERVER_URL"]
ES_ENDPOINT = os.environ["ES_ENDPOINT"]
ES_USERNAME = os.environ["ES_USERNAME"]
ES_PASSWORD = os.environ["ES_PASSWORD"]
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")
config.load_incluster_config()
configuration = kubernetes.client.Configuration()
api_instance = kubernetes.client.BatchV1Api()
logging.info("Cluster config has been been loaded")

nulog_spec = {
    "name": "nulog-train",
    "container_name": "nulog-train",
    "image_name": "amartyarancher/nulog-train:v0.1",
    "image_pull_policy": "Always",
    "labels": {"app": "nulog-train"},
    "restart_policy": "Never",
    "requests": {"memory": "1Gi", "cpu": 1},
    "limits": {"nvidia.com/gpu": 1},
    "env": [],
}
nulog_spec["env"] = [
    client.V1EnvVar(name="MINIO_SERVER_URL", value=MINIO_SERVER_URL),
    client.V1EnvVar(name="MINIO_ACCESS_KEY", value=MINIO_ACCESS_KEY),
    client.V1EnvVar(name="MINIO_SECRET_KEY", value=MINIO_SECRET_KEY),
    client.V1EnvVar(name="NATS_SERVER_URL", value=NATS_SERVER_URL),
]
startup_time = time.time()

NAMESPACE = os.environ["JOB_NAMESPACE"]
DEFAULT_TRAINING_INTERVAL = 1800  # 1800 seconds aka 30mins

es = AsyncElasticsearch(
    [ES_ENDPOINT],
    port=9200,
    http_auth=(ES_USERNAME, ES_PASSWORD),
    verify_certs=False,
    use_ssl=True,
)


async def update_es_job_status(
    request_id: str,
    job_status: str,
    op_type: str = "update",
    index: str = "training_signal",
):
    """
    this method updates the status of jobs in elasticsearch.
    """
    script = "ctx._source.status = '{}';".format(job_status)
    docs_to_update = [
        {
            "_id": request_id,
            "_op_type": op_type,
            "_index": index,
            "script": script,
        }
    ]
    logging.info("ES job {} status update : {}".format(request_id, job_status))
    try:
        async for ok, result in async_streaming_bulk(es, docs_to_update):
            action, result = result.popitem()
            if not ok:
                logging.error("failed to %s document %s" % ())
    except Exception as e:
        logging.error(e)


async def es_training_signal_coroutine(signals_queue: asyncio.Queue):
    """
    collect job training signal from elasticsearch, and add to job queue.
    """
    query_body = {"query": {"bool": {"must": {"match": {"status": "submitted"}}}}}
    index = "training_signal"
    current_time = int(datetime.timestamp(datetime.now()))
    job_payload = {
        "model_to_train": "nulog",
        "time_intervals": [
            {
                "start_ts": (current_time - DEFAULT_TRAINING_INTERVAL) * (10 ** 9),
                "end_ts": current_time * (10 ** 9),
            }
        ],
    }
    signal_index_exists = False
    try:
        signal_index_exists = await es.indices.exists(index)
        if not signal_index_exists:
            signal_created = await es.indices.create(index=index)
    except exceptions.TransportError as e:
        logging.error(e)
    while True:
        try:
            user_signals_response = await es.search(
                index=index, body=query_body, size=100
            )
            user_signal_hits = user_signals_response["hits"]["hits"]
            if len(user_signal_hits) > 0:
                for hit in user_signal_hits:
                    signals_queue_payload = {
                        "source": "elasticsearch",
                        "_id": hit["_id"],
                        "model": "nulog-train",
                        "signal": "start",
                        "payload": job_payload,
                    }
                    await update_es_job_status(
                        request_id=hit["_id"], job_status="scheduled"
                    )
                    await signals_queue.put(signals_queue_payload)
        except (exceptions.NotFoundError, exceptions.TransportError) as e:
            logging.error(e)

        await asyncio.sleep(60)


def job_not_currently_running(job_name, namespace=NAMESPACE):
    try:
        jobs = api_instance.list_namespaced_job(namespace, timeout_seconds=60)
        for job in jobs.items:
            if job.metadata.name == job_name:
                return False
        return True
    except ApiException as e:
        logging.error(
            "Exception when calling BatchV1Api->list_namespaced_job: %s\n" % e
        )


async def kube_delete_empty_pods(signals_queue, namespace=NAMESPACE, phase="Succeeded"):
    deleteoptions = client.V1DeleteOptions()
    # We need the api entry point for pods
    api_pods = client.CoreV1Api()
    # List the pods
    try:
        pods = api_pods.list_namespaced_pod(namespace, timeout_seconds=60)
        for pod in pods.items:
            podname = pod.metadata.name
            try:
                if pod.status.phase == phase:
                    api_response = api_pods.delete_namespaced_pod(
                        podname, namespace, body=deleteoptions
                    )
                    if "nulog-train" in podname:
                        signals_queue_payload = {
                            "model": "nulog-train",
                            "signal": "finish",
                            "payload": None,
                        }
                        await signals_queue.put(signals_queue_payload)
                else:
                    logging.info(
                        "Pod: {} still not done... Phase: {}".format(
                            podname, pod.status.phase
                        )
                    )
            except ApiException as e:
                logging.error(
                    "Exception when calling CoreV1Api->delete_namespaced_pod: %s\n" % e
                )
    except ApiException as e:
        logging.error("Exception when calling CoreV1Api->list_namespaced_pod: %s\n" % e)

    return


def run_job(job_details):
    resource_specifications = client.V1ResourceRequirements(
        requests=job_details["requests"], limits=job_details["limits"]
    )
    container = client.V1Container(
        name=job_details["container_name"],
        image=job_details["image_name"],
        image_pull_policy=job_details["image_pull_policy"],
        env=job_details["env"],
        resources=resource_specifications,
    )
    template = client.V1PodTemplateSpec(
        metadata=client.V1ObjectMeta(labels=job_details["labels"]),
        spec=client.V1PodSpec(
            restart_policy=job_details["restart_policy"], containers=[container]
        ),
    )
    spec = client.V1JobSpec(template=template, backoff_limit=4)
    job = client.V1Job(
        api_version="batch/v1",
        kind="Job",
        metadata=client.V1ObjectMeta(name=job_details["name"]),
        spec=spec,
    )
    api_instance.create_namespaced_job(body=job, namespace=NAMESPACE)


async def clear_jobs(signals_queue):
    namespace = NAMESPACE
    state = "finished"
    while True:
        await asyncio.sleep(300)
        deleteoptions = client.V1DeleteOptions()
        try:
            jobs = api_instance.list_namespaced_job(namespace, timeout_seconds=60)
            # Now we have all the jobs, lets clean up
            # We are also logging the jobs we didn't clean up because they either failed or are still running
            for job in jobs.items:
                jobname = job.metadata.name
                jobstatus = job.status.conditions
                if job.status.succeeded == 1:
                    # Clean up Job
                    logging.info(
                        "Cleaning up Job: {}. Finished at: {}".format(
                            jobname, job.status.completion_time
                        )
                    )
                    try:
                        # What is at work here. Setting Grace Period to 0 means delete ASAP. Otherwise it defaults to
                        # some value I can't find anywhere. Propagation policy makes the Garbage cleaning Async
                        api_response = api_instance.delete_namespaced_job(
                            jobname,
                            namespace,
                            body=deleteoptions,
                            grace_period_seconds=0,
                            propagation_policy="Background",
                        )
                    except ApiException as e:
                        logging.error(
                            "Exception when calling BatchV1Api->delete_namespaced_job: %s\n"
                            % e
                        )
                else:
                    if jobstatus is None and job.status.active == 1:
                        jobstatus = "active"
                    logging.info(
                        "Job: {} not cleaned up. Current status: {}".format(
                            jobname, jobstatus
                        )
                    )
        except ApiException as e:
            logging.error(
                "Exception when calling BatchV1Api->list_namespaced_job: %s\n" % e
            )

        # Now that we have the jobs cleaned, let's clean the pods
        await kube_delete_empty_pods(signals_queue)


async def manage_kubernetes_training_jobs(signals_queue):
    nulog_next_job_to_run = None  ## TODO: (minor) should replace this with a queue? in case there are multiple pending jobs
    while True:
        payload = await signals_queue.get()
        if payload is None:
            break
        signal = payload["signal"]
        model_to_train = payload["model"]
        model_payload = payload["payload"]
        if signal == "start":
            if job_not_currently_running(model_to_train):
                PrepareTrainingLogs("/tmp").run(model_payload["time_intervals"])
                if model_to_train == "nulog-train":
                    if "source" in payload and payload["source"] == "elasticsearch":
                        await update_es_job_status(
                            request_id=payload["_id"], job_status="trainingStarted"
                        )
                    run_job(nulog_spec)
                    ## es_update_status = training_inprogress
            else:
                if model_to_train == "nulog-train":
                    nulog_next_job_to_run = model_payload
                    logging.info(
                        "Nulog model currently being trained. Job will run after this model's training has been completed"
                    )
                    if "source" in payload and payload["source"] == "elasticsearch":
                        await update_es_job_status(
                            request_id=payload["_id"], job_status="pendingInQueue"
                        )
                    ## es_update_status = pending_in_queue
        else:
            if nulog_next_job_to_run:
                PrepareTrainingLogs("/tmp").run(nulog_next_job_to_run["time_intervals"])
                run_job(nulog_spec)
                nulog_next_job_to_run = None


async def consume_nats_drain_signal(queue, signals_queue):
    while True:
        payload = await queue.get()
        if payload is None:
            break
        try:
            decoded_payload = json.loads(payload)
            # process the payload
            if decoded_payload["model_to_train"] == "nulog":
                signals_queue_payload = {
                    "model": "nulog-train",
                    "signal": "start",
                    "payload": decoded_payload,
                }
                await signals_queue.put(signals_queue_payload)

            logging.info("Just received signal to begin running the jobs")

        except Exception as e:
            logging.error(e)


async def consume_payload_coroutine(jobs_queue):
    nw = NatsWrapper()
    await nw.connect()
    await nw.subscribe(nats_subject="train", payload_queue=jobs_queue)


if __name__ == "__main__":
    loop = asyncio.get_event_loop()
    jobs_queue = asyncio.Queue(loop=loop)
    signals_queue = asyncio.Queue(loop=loop)
    consumer_coroutine = consume_payload_coroutine(jobs_queue)
    consume_nats_drain_signal_coroutine = consume_nats_drain_signal(
        jobs_queue, signals_queue
    )
    clear_jobs_coroutine = clear_jobs(signals_queue)
    manage_kubernetes_jobs_coroutine = manage_kubernetes_training_jobs(signals_queue)
    es_signal_coroutine = es_training_signal_coroutine(signals_queue)
    loop.run_until_complete(
        asyncio.gather(
            consumer_coroutine,
            consume_nats_drain_signal_coroutine,
            clear_jobs_coroutine,
            manage_kubernetes_jobs_coroutine,
            es_signal_coroutine,
        )
    )
    try:
        loop.run_forever()
    finally:
        loop.close()
