import asyncio
import json
from typing import List
from datetime import datetime, timedelta

from grpclib.client import Channel
from cortexadmin_pb import CortexAdminStub, SeriesRequest, QueryRangeRequest, QueryRequest, MatcherRequest
from betterproto.lib.google.protobuf import Empty

default_query_interval = "1m"

async def get_all_users(service: CortexAdminStub) -> List[str]:
  response = await service.all_user_stats(Empty())
  return [r.user_id for r in response.items]

async def list_all_metric(service: CortexAdminStub, cluster_id: str) -> List[str]:
  response = await service.extract_raw_series(MatcherRequest(tenant=cluster_id, match_expr=".+"))
  res = (json.loads(response.data.decode())["data"])
  s = set()
  for r in res["result"]:
    s.add(r["metric"]["__name__"])
  return list(s)

async def metric_query(service: CortexAdminStub, cluster_id: str, metric_name: str, namespace="opni"):
  query = f'sum(rate({metric_name}{{namespace="{namespace}"}}[{default_query_interval}])) by (pod)'
  response = await service.query(QueryRequest(tenants=[cluster_id], query=query))
  response = json.loads(response.data.decode())["data"]
  return response

async def metric_queryrange(service: CortexAdminStub, cluster_id: str, metric_name: str, namespace="opni", end_time : datetime = None, time_delta : timedelta= timedelta(minutes=60), step_minute : int = 1):
  query_interval = "2m"# f"{step_minute}m"
  query = f'sum(rate({metric_name}{{namespace="{namespace}"}}[{query_interval}])) by (pod)'
  if end_time is None:
    end_time = datetime.now()
  start_time = end_time - time_delta
  response = await service.query_range(QueryRangeRequest(tenants=[cluster_id], query=query, start=start_time, end=end_time, step=timedelta(minutes=step_minute)))
  response = json.loads(response.data.decode())["data"]
  return response

async def main():
  channel = Channel(host=OPNI_HOST, port=11090) # url of opni-internal. can port-forward to localhost:11090
  service = CortexAdminStub(channel)
  # response = await service.get_cortex_status(Empty())
  user_id = (await get_all_users(service))[0]
  metrics = await list_all_metric(service, user_id)
  m_name = "container_cpu_usage_seconds_total"
  q1 = await metric_query(service, user_id, m_name)  
  q2 = await metric_queryrange(service, user_id, m_name)

  channel.close()



if __name__ == "__main__":
    # test()
    asyncio.new_event_loop().run_until_complete(main())