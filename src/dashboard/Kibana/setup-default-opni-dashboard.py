# Standard Library
import json
import os

# Third Party
from elasticsearch import Elasticsearch

ES_ENDPOINT = os.environ["ES_ENDPOINT"]
ES_USER = os.getenv("ES_USER", "admin")
ES_PASSWORD = os.getenv("ES_PASSWORD", "admin")

es = Elasticsearch(
    [ES_ENDPOINT],
    port=9200,
    http_compress=True,
    http_auth=(ES_USER, ES_PASSWORD),
    verify_certs=False,
    use_ssl=True,
)

# res = (es.search(index=".kibana_92668751_admin_1"))["hits"]["hits"]

with open("default-opni-dasahboard.json", "r") as fin:
    res = json.load(fin)
    for r in res:
        es.index(index=r["_index"], id=r["_id"], body=r["_source"], doc_type=r["_type"])
