import asyncio

from grpclib.client import Channel
from cortexadmin_pb import CortexAdminStub, SeriesRequest, QueryRangeRequest, QueryRequest
from betterproto.lib.google.protobuf import Empty


async def main():
  channel = Channel(host="localhost", port=11090) # url of opni-internal. can port-forward to localhost:11090
  service = CortexAdminStub(channel)
  # response = await service.get_cortex_status(Empty())
  # user_id, cluster_id , tenant_id all the same thing :)
  response = await service.all_user_stats(Empty())
  user_id = "8d043f30-4e38-4b2e-8d1c-a48e13146c54"
  response = await service.query(QueryRequest(tenants=[user_id], query="cpu_allocated"))
  response = await service.get_series_metrics(SeriesRequest(tenant=user_id))
  print(response)

  channel.close()

if __name__ == "__main__":
  asyncio.new_event_loop().run_until_complete(main())