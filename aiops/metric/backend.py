from fastapi import FastAPI, BackgroundTasks
from access_admin_api import get_all_users, list_all_metric
from grpclib.client import Channel
from cortexadmin_pb import CortexAdminStub
import time
from datetime import datetime
from filter_anomaly_metric import get_abnormal_metrics

OPNI_HOST = "localhost" # opni-internal

storage = {}
app = FastAPI()


@app.get("/")
async def root():
    return {"message": "Hello World"}


@app.get("/get_users")
async def get_users():
    channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
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


async def func_get_metrics(user_id):
    channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    res = await list_all_metric(service, user_id)
    storage["task_id"] = res
    channel.close()
    return res

async def func_get_abnormal_metrics(user_id, requested_ts= None, ns="default"):
    channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    res = await get_abnormal_metrics(service, user_id, requested_ts, ns)
    channel.close()
    storage["task_id"] = res
    return res

@app.post("/create_task/{user_id}")
async def create_task(user_id, background_tasks: BackgroundTasks):
    ts = datetime.now()
    background_tasks.add_task(func_get_abnormal_metrics, user_id, ts)  
    return {"submitted time": ts, "retrive_id" :"task_id"}

@app.get("/get_task_res/{task_id}")
def get_task_res(task_id):
    if task_id in storage:
        return storage[task_id]
    else:
        return {"Not exist!"}