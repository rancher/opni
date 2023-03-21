from fastapi import FastAPI, BackgroundTasks
from access_admin_api import get_all_users, list_all_metric
from grpclib.client import Channel
from cortexadmin_pb import CortexAdminStub
import time
from datetime import datetime
from filter_anomaly_metric import get_abnormal_metrics
from model.metric_pattern_classification import predict
from uuid import uuid4
import os
from opni_nats import NatsWrapper
import json

OPNI_HOST = os.getenv("OPNI_HOST", "localhost") # opni-internal
OPNI_PORT = 11090
BUCKET_NAME = "metric_results"

nw = NatsWrapper()
app = FastAPI()


@app.get("/")
async def root():
    return {"message": "Opni Metric Analysis Backend API"}


@app.on_event("startup")
async def startup_event():
    """
    startup even: connect to nats, create bucket if not exists
    """
    await nw.connect()
    for b in [BUCKET_NAME]:
        bucket_exists = await nw.get_bucket(b)
        if bucket_exists is None:
            await nw.create_bucket(b)


@app.get("/get_users")
async def get_users():
    channel = Channel(host=OPNI_HOST, port=OPNI_PORT) # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    res = await get_all_users(service)
    channel.close()
    return res


@app.get("/get_metrics/{user_id}")
async def get_metrics(user_id):
    channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    res = await list_all_metric(service, user_id)
    channel.close()
    return res


@app.get("/list_requests")
async def list_requests():
    kv = await nw.get_bucket(BUCKET_NAME)
    keys = await kv.kv.keys()
    return [k for k in keys]


@app.post("/create_task/{user_id}/{namespace}")
async def create_task(user_id, namespace, background_tasks: BackgroundTasks):
    ts = datetime.now()
    task_id = str(uuid4())
    background_tasks.add_task(func_get_abnormal_metrics,task_id, user_id, ts, namespace)  
    kv = await nw.get_bucket(BUCKET_NAME)
    status = {"status" : "task submitted", "task_id": task_id, "submitted_time": ts.timestamp()}
    await kv.put(key=task_id, value=json.dumps(status).encode())
    return {"submitted time": ts, "retrive_id" :task_id}

@app.get("/get_task_res/{task_id}")
async def get_task_res(task_id):
    kv = await nw.get_bucket(BUCKET_NAME)
    task_res=await kv.get(key=task_id)
    if  task_res is not None:
        return json.loads(task_res.decode())
    else:
        return {"The requested task does not exist!"}
    

# res = await nw.get_bucket("model-training-parameters")
# bucket_payload = await res.get("modelTrainingParameters")
# workload_parameters_dict = json.loads(bucket_payload.decode())

async def func_get_metrics(user_id):
    channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    res = await list_all_metric(service, user_id)
    # storage["task_id"] = res
    channel.close()
    return res

async def func_get_abnormal_metrics(task_id, user_id, requested_ts= None, ns="default"):
    channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    anomaly_metrics = await get_abnormal_metrics(service, user_id, requested_ts, ns)
    anomaly_metrics_value = [v for p,m,v in anomaly_metrics]
    preds = predict(anomaly_metrics_value)
    channel.close()
    res = {}
    for i, (p,m,v) in enumerate(anomaly_metrics):
        res[p+"-"+m] = preds[i]
    
    kv = await nw.get_bucket(BUCKET_NAME)
    current_status = json.loads((await kv.get(task_id)).decode())
    current_status["result"] = res
    current_status["status"] = "task completed"
    await kv.put(key=task_id, value=json.dumps(current_status).encode())
    return res