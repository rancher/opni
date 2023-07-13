"""
This script implement helper functions that collect useful metrics by querying Cortex API exposed by Opni Gateway.
"""
# Standard Library
import asyncio
import json
from datetime import datetime, timedelta
from typing import List

# Third Party
from betterproto.lib.google.protobuf import Empty
from cortexadmin_pb import (
    CortexAdminStub,
    MatcherRequest,
    QueryRangeRequest,
    QueryRequest,
    SeriesRequest,
)
from envvars import OPNI_GATEWAY_HOST, OPNI_GATEWAY_PORT
from grpclib.client import Channel


async def get_all_users(service: CortexAdminStub) -> List[str]:
    """
    list cluster_ids, although in the API they are known as `user_id`
    """
    response = await service.all_user_stats(Empty())
    return [r.user_id for r in response.items]


async def list_namespace(service: CortexAdminStub, cluster_id: str):
    """
    list namespaces of a given cluster_id
    """
    query = "kube_namespace_labels"
    response = await service.query(QueryRequest(tenants=[cluster_id], query=query))
    response = json.loads(response.data.decode())["data"]
    res = [i["metric"]["namespace"] for i in response["result"]]
    return res


async def list_ns_pod(service: CortexAdminStub, cluster_id: str, namespace: str):
    """
    list pods in a given namespace of a given cluster_id
    """
    query = f'kube_pod_labels{{namespace="{namespace}"}}'
    response = await service.query(QueryRequest(tenants=[cluster_id], query=query))
    response = json.loads(response.data.decode())["data"]
    res = [i["metric"]["pod"] for i in response["result"]]
    return res


async def list_ns_service(service: CortexAdminStub, cluster_id: str, namespace: str):
    """
    list services in a given namespace of a given cluster_id
    """
    query = f'kube_service_labels{{namespace="{namespace}"}}'
    response = await service.query(QueryRequest(tenants=[cluster_id], query=query))
    response = json.loads(response.data.decode())["data"]
    res = [i["metric"]["service"] for i in response["result"]]
    return res


async def list_all_metric(service: CortexAdminStub, cluster_id: str) -> List[str]:
    """
    helper function that list all metrics of a given cluster_id
    """
    response = await service.extract_raw_series(
        MatcherRequest(tenant=cluster_id, match_expr=".+")
    )
    res = json.loads(response.data.decode())["data"]
    s = set()
    for r in res["result"]:
        s.add(r["metric"]["__name__"])
    return list(s)

def get_query(metric_name: str, query_interval:str = "2m", namespace: str= None, aggregate:str = "by (pod)"):
    """
    form a query given certain information
    """
    if namespace is None:
        res_query = f'sum(rate({metric_name}[{query_interval}])) ' + aggregate
    else:
        res_query = f'sum(rate({metric_name}{{namespace="{namespace}"}}[{query_interval}])) ' + aggregate
    return res_query


async def metric_query(
    service: CortexAdminStub, cluster_id: str, metric_name: str, namespace="opni"
):
    """
    query
    """
    query = get_query(metric_name, namespace=namespace)
    response = await service.query(QueryRequest(tenants=[cluster_id], query=query))
    response = json.loads(response.data.decode())["data"]
    return response


async def metric_queryrange(
    service: CortexAdminStub,
    cluster_id: str,
    metric_name: str,
    namespace="opni",
    end_time: datetime = None,
    time_delta: timedelta = timedelta(minutes=60),
    step_minute: int = 1,
):
    """
    query_range. As first iteration, collects pod-level metrics.
    param:
      @time_delta: defines how far it looks back, default value is timedelta(minutes=60) which means last 60 mins
      @step_minute: defines step in the unit `minute`, default value is 1, indicate it will collect data points every 1 min
      @end_time: the timestamp to trace back.
    """
    query = get_query(metric_name, namespace=namespace)
    if end_time is None:
        end_time = datetime.now()
    start_time = end_time - time_delta
    response = await service.query_range(
        QueryRangeRequest(
            tenants=[cluster_id],
            query=query,
            start=start_time,
            end=end_time,
            step=timedelta(minutes=step_minute),
        )
    )
    response = json.loads(response.data.decode())["data"]
    return response


async def main():
    channel = Channel(
        host=OPNI_GATEWAY_HOST, port=OPNI_GATEWAY_PORT
    )  # url of opni-internal. can port-forward to localhost:11090
    service = CortexAdminStub(channel)
    # response = await service.get_cortex_status(Empty())
    user_id = (await get_all_users(service))[0]
    metrics = await list_all_metric(service, user_id)
    m_name = "container_cpu_usage_seconds_total"
    q1 = await metric_query(service, user_id, m_name)
    q2 = await metric_queryrange(service, user_id, m_name)

    channel.close()


if __name__ == "__main__":
    asyncio.new_event_loop().run_until_complete(main())
