# Standard Library
import json
from dataclasses import asdict, dataclass
from datetime import datetime
from typing import List
import requests
import logging
import asyncio

# Third Party
from access_admin_api import (
    get_all_users,
    list_all_metric,
    list_namespace,
    list_ns_pod,
    list_ns_service,
    get_query
)
from cortexadmin_pb import CortexAdminStub
from envvars import OPNI_GATEWAY_HOST, OPNI_GATEWAY_PORT, OPNI_GATEWAY_PLUGINAPI_PORT
from fastapi import BackgroundTasks, FastAPI
from filter_anomaly_metric import get_abnormal_metrics
from grpclib.client import Channel
from model.metric_pattern_classification import predict
from opni_nats import NatsWrapper
from grafana_dashboard_utils import get_grafana_dashboard_payload

BUCKET_NAME = "metric_ai_jobs"
BUCKET_NAME_RUNS = "metric_ai_job_runs"
JOB_RUN_DELIMITER = "=" # use this to seperate job_id and suffix, so we can identify properly jobrun_id in natsKV
JOBRUN_STATUS_SUBMITTED = "Job Run Submitted"
JOBRUN_STATUS_COMPLETED = "Job Run Completed"

create_dashboard_url = f"http://{OPNI_GATEWAY_HOST}:{OPNI_GATEWAY_PLUGINAPI_PORT}/MetricAI/metricai/creategrafanadashboard"

nw = NatsWrapper()
app = FastAPI()


@app.get("/")
async def root():
    return {"message": "Opni Metric Analysis Backend API"}


@app.get("/healthcheck")
def healthcheck():
    return {"healthcheck": "Everything OK!"}


@app.on_event("startup")
async def startup_event():
    """
    startup even: connect to nats, create natsKV bucket if not exists
    """
    await nw.connect()
    for b in [BUCKET_NAME, BUCKET_NAME_RUNS]:
        bucket_exists = await nw.get_bucket(b)
        if bucket_exists is None:
            await nw.create_bucket(b)


@app.get("/get_users")
async def get_users():
    channel = Channel(host=OPNI_GATEWAY_HOST, port=OPNI_GATEWAY_PORT)
    service = CortexAdminStub(channel)
    res = await get_all_users(service)
    channel.close()
    return res


@app.get("/get_metrics/{cluster_id}")
async def get_metrics(cluster_id):
    channel = Channel(host=OPNI_GATEWAY_HOST, port=OPNI_GATEWAY_PORT)
    service = CortexAdminStub(channel)
    res = await list_all_metric(service, cluster_id)
    channel.close()
    return res


@app.get("/list_namespace/{cluster_id}")
async def list_cluster_namespace(cluster_id):
    channel = Channel(host=OPNI_GATEWAY_HOST, port=OPNI_GATEWAY_PORT)
    service = CortexAdminStub(channel)
    res = await list_namespace(service, cluster_id)
    channel.close()
    return res


@app.get("/list_ns_service/{cluster_id}/{namespace}")
async def list_cluster_ns_service(cluster_id, namespace):
    channel = Channel(host=OPNI_GATEWAY_HOST, port=OPNI_GATEWAY_PORT)
    service = CortexAdminStub(channel)
    res = await list_ns_service(service, cluster_id, namespace)
    channel.close()
    return res


@app.get("/list_jobs")
async def list_jobs():
    kv = await nw.get_bucket(BUCKET_NAME)
    keys = await kv.kv.keys()
    return [k for k in keys]


@app.get("/list_job_runs/{job_id}")
async def list_job_runs(job_id):
    kv = await nw.get_bucket(BUCKET_NAME_RUNS)
    keys = await kv.kv.keys()
    return [k for k in keys if k.startswith(job_id + JOB_RUN_DELIMITER)]


@app.get("/get_job/{job_id}")
async def get_job(job_id: str):
    kv = await nw.get_bucket(BUCKET_NAME)
    res = await kv.get(job_id)
    return json.loads(res.decode())


@app.get("/get_job_run/{job_run_id}")
async def get_job_run(job_run_id: str):
    kv = await nw.get_bucket(BUCKET_NAME_RUNS)
    res = await kv.get(job_run_id)
    return json.loads(res.decode())


# TODO: replace these dataclass with the auto-generated dataclass from proto in the aiops gateway plugin


@dataclass
class jobRunStatus:
    JobId: str
    JobRunId: str
    JobRunResult: str
    JobRunCreateTime: str
    JobRunBaseTime: str
    Status: str
    JobRunResultDetails: str


@dataclass
class jobRunResponse:
    JobRunId: str
    Status: str
    SubmittedTime: str


@dataclass
class jobStatus:
    JobId: str
    JobCreateTime: str
    ClusterId: str
    Namespaces: List[str]
    JobDescription: str


@app.get("/run_job/{job_id}/")
async def run_job(job_id, background_tasks: BackgroundTasks):
    ts = datetime.now()
    kv = await nw.get_bucket(BUCKET_NAME)
    job_meta = json.loads((await kv.get(job_id)).decode())
    job_meta = jobStatus(**job_meta)
    cluster_id = job_meta.ClusterId
    namespaces = job_meta.Namespaces

    jobrun_id = (
        job_id + JOB_RUN_DELIMITER + ts.strftime("t%Y%m%d%H%M%S%f")
    ).lower()  # use ts as unique suffix
    background_tasks.add_task(
        func_get_abnormal_metrics, jobrun_id, cluster_id, ts, namespaces
    )  # run as background task

    kv_run = await nw.get_bucket(BUCKET_NAME_RUNS)
    str_ts = str(ts.timestamp())
    status = jobRunStatus(
        JobId=job_id,
        JobRunId=jobrun_id,
        JobRunBaseTime=str_ts,
        JobRunCreateTime=str_ts,
        Status=JOBRUN_STATUS_SUBMITTED,
        JobRunResult="",
        JobRunResultDetails="",
    )
    status = asdict(status)
    await kv_run.put(
        key=jobrun_id, value=json.dumps(status).encode()
    )  # update jobrun status

    response = jobRunResponse(
        JobRunId=jobrun_id, Status="Success", SubmittedTime=str_ts
    )
    response = asdict(response)
    return response


async def func_get_abnormal_metrics(jobrun_id, cluster_id, requested_ts=None, nss=[]):
    channel = Channel(host=OPNI_GATEWAY_HOST, port=OPNI_GATEWAY_PORT)
    service = CortexAdminStub(channel)
    res = {}
    dashboard_payload_info = []
    anomaly_count, total_count = 0, 0
    # get anomalous metrics and match pattern
    for ns in nss:
        anomaly_metric_list, all_metric_list = await get_abnormal_metrics(
            service, cluster_id, requested_ts, ns
        )
        anomaly_count += len(anomaly_metric_list)
        total_count += len(all_metric_list)
        anomaly_metrics_value = [values for pod, metric_name, values in anomaly_metric_list]
        preds = predict(anomaly_metrics_value)
        
        for i, (pod, metric_name, values) in enumerate(anomaly_metric_list):
            res[pod + JOB_RUN_DELIMITER + metric_name] = preds[i]
            dashboard_payload_info.append((preds[i], pod + metric_name, get_query(metric_name, namespace=ns)))
    channel.close()
    
    if anomaly_count > 0:
        legal_dashboard_id = jobrun_id.replace(JOB_RUN_DELIMITER,"").lower()
        dashboard_payload = get_grafana_dashboard_payload(dashboard_payload_info, legal_dashboard_id)
        
        dashboard_payload = json.dumps(dashboard_payload)
    else:
        dashboard_payload = ""
    # update jobrun status to natsKV
    kv = await nw.get_bucket(BUCKET_NAME_RUNS)
    current_status = json.loads((await kv.get(jobrun_id)).decode())
    current_status = jobRunStatus(**current_status)
    current_status.JobRunResult = (
        f"Scanned {total_count} metrics, Anomalous metrics: {anomaly_count}"
    )
    current_status.JobRunResultDetails = dashboard_payload
    current_status.Status = JOBRUN_STATUS_COMPLETED
    current_status = asdict(current_status)
    await kv.put(key=jobrun_id, value=json.dumps(current_status).encode())   

    if anomaly_count > 0:
        try:
            await asyncio.sleep(2)
            url = create_dashboard_url + "/" + jobrun_id
            response = requests.post(url)
        except Exception as e:
            logging.error(f"failed to create dashboard, Error : {e}")
    return res
