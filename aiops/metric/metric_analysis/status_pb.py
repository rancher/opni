
from __future__ import annotations
from dataclasses import dataclass

from datetime import datetime


from typing import List

import betterproto




@dataclass(eq=False, repr=False)
class CortexStatus(betterproto.Message):
    distributor: DistributorStatus = betterproto.message_field(1)
    ingester: IngesterStatus = betterproto.message_field(2)
    ruler: RulerStatus = betterproto.message_field(3)
    purger: PurgerStatus = betterproto.message_field(4)
    compactor: CompactorStatus = betterproto.message_field(5)
    store_gateway: StoreGatewayStatus = betterproto.message_field(6)
    query_frontend: QueryFrontendStatus = betterproto.message_field(7)
    querier: QuerierStatus = betterproto.message_field(8)
    timestamp: datetime = betterproto.message_field(9)


@dataclass(eq=False, repr=False)
class ServiceStatus(betterproto.Message):
    """ Status of an individual cortex service (module) """
    name: str = betterproto.string_field(1)
    status: str = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class ServiceStatusList(betterproto.Message):
    services: List[ServiceStatus] = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class ShardStatus(betterproto.Message):
    """ Status of a single shard in a ring """
    id: str = betterproto.string_field(1)
    state: str = betterproto.string_field(2)
    address: str = betterproto.string_field(3)
    timestamp: str = betterproto.string_field(4)
    registered_timestamp: str = betterproto.string_field(5)
    zone: str = betterproto.string_field(6)


@dataclass(eq=False, repr=False)
class ShardStatusList(betterproto.Message):
    shards: List[ShardStatus] = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class MemberStatus(betterproto.Message):
    """ Status of a single member of a memberlist """
    name: str = betterproto.string_field(1)
    address: str = betterproto.string_field(2)
    port: int = betterproto.uint32_field(3)
    state: int = betterproto.int32_field(4)


@dataclass(eq=False, repr=False)
class MemberStatusList(betterproto.Message):
    items: List[MemberStatus] = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class MemberlistStatus(betterproto.Message):
    enabled: bool = betterproto.bool_field(1)
    """ Whether the service is currently using a memberlist """
    members: MemberStatusList = betterproto.message_field(2)
    """ The status of each member in the memberlist """
    keys: List[str] = betterproto.string_field(3)
    """ A list of keys in the key-value store used by the memberlist """


@dataclass(eq=False, repr=False)
class RingStatus(betterproto.Message):
    enabled: bool = betterproto.bool_field(1)
    shards: ShardStatusList = betterproto.message_field(2)


@dataclass(eq=False, repr=False)
class DistributorStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)
    ingester_ring: RingStatus = betterproto.message_field(2)


@dataclass(eq=False, repr=False)
class IngesterStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)
    memberlist: MemberlistStatus = betterproto.message_field(2)
    ring: RingStatus = betterproto.message_field(3)


@dataclass(eq=False, repr=False)
class RulerStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)
    memberlist: MemberlistStatus = betterproto.message_field(2)
    ring: RingStatus = betterproto.message_field(3)


@dataclass(eq=False, repr=False)
class PurgerStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class CompactorStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)
    memberlist: MemberlistStatus = betterproto.message_field(2)
    ring: RingStatus = betterproto.message_field(3)


@dataclass(eq=False, repr=False)
class StoreGatewayStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)
    memberlist: MemberlistStatus = betterproto.message_field(2)
    ring: RingStatus = betterproto.message_field(3)


@dataclass(eq=False, repr=False)
class QueryFrontendStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class QuerierStatus(betterproto.Message):
    services: ServiceStatusList = betterproto.message_field(1)
    memberlist: MemberlistStatus = betterproto.message_field(2)



