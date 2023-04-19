# Standard Library
import os

OPNI_GATEWAY_HOST = os.getenv("OPNI_GATEWAY_HOST", "localhost")  # opni-internal
OPNI_GATEWAY_PORT = int(os.getenv("OPNI_GATEWAY_PORT", 11090))
