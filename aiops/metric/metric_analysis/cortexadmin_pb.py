
from __future__ import annotations
from dataclasses import dataclass

from datetime import datetime, timedelta


from typing import Dict, List, Optional

import betterproto
from status_pb import CortexStatus
import betterproto.lib.google.protobuf as betterproto_lib_google_protobuf
from betterproto.grpc.grpclib_server import ServiceBase
import grpclib
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from betterproto.grpc.grpclib_client import MetadataLike
    from grpclib.metadata import Deadline


class MetricMetadataMetricType(betterproto.Enum):
    UNKNOWN = 0
    COUNTER = 1
    GAUGE = 2
    HISTOGRAM = 3
    GAUGEHISTOGRAM = 4
    SUMMARY = 5
    INFO = 6
    STATESET = 7


@dataclass(eq=False, repr=False)
class Cluster(betterproto.Message):
    cluster_id: str = betterproto.string_field(1)


@dataclass(eq=False, repr=False)
class SeriesRequest(betterproto.Message):
    tenant: str = betterproto.string_field(1)
    job_id: str = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class MatcherRequest(betterproto.Message):
    tenant: str = betterproto.string_field(1)
    match_expr: str = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class LabelRequest(betterproto.Message):
    tenant: str = betterproto.string_field(1)
    job_id: str = betterproto.string_field(2)
    metric_name: str = betterproto.string_field(3)


@dataclass(eq=False, repr=False)
class MetricLabels(betterproto.Message):
    items: List[LabelSet] = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class LabelSet(betterproto.Message):
    name: str = betterproto.string_field(1)
    items: List[str] = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class SeriesMetadata(betterproto.Message):
    description: str = betterproto.string_field(1)
    type: str = betterproto.string_field(2)
    unit: str = betterproto.string_field(3)


@dataclass(eq=False, repr=False)
class SeriesInfo(betterproto.Message):
    series_name: str = betterproto.string_field(1)
    metadata: SeriesMetadata = betterproto.message_field(2)


@dataclass(eq=False, repr=False)
class SeriesInfoList(betterproto.Message):
    items: List[SeriesInfo] = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class UserIDStatsList(betterproto.Message):
    items: List[UserIDStats] = betterproto.message_field(2)


@dataclass(eq=False, repr=False)
class UserIDStats(betterproto.Message):
    user_id: str = betterproto.string_field(1)
    ingestion_rate: float = betterproto.double_field(2)
    num_series: int = betterproto.uint64_field(3)
    api_ingestion_rate: float = betterproto.double_field(4)
    rule_ingestion_rate: float = betterproto.double_field(5)


@dataclass(eq=False, repr=False)
class WriteRequest(betterproto.Message):
    cluster_id: str = betterproto.string_field(1)
    timeseries: List[TimeSeries] = betterproto.message_field(2)
    metadata: List[MetricMetadata] = betterproto.message_field(3)


@dataclass(eq=False, repr=False)
class MetricMetadataRequest(betterproto.Message):
    tenants: List[str] = betterproto.string_field(1)
    metric_name: str = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class WriteResponse(betterproto.Message):
    pass


@dataclass(eq=False, repr=False)
class TimeSeries(betterproto.Message):
    labels: List[Label] = betterproto.message_field(1)
    samples: List[Sample] = betterproto.message_field(2)
    exemplars: List[Exemplar] = betterproto.message_field(3)


@dataclass(eq=False, repr=False)
class Label(betterproto.Message):
    name: str = betterproto.string_field(1)
    value: str = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class Sample(betterproto.Message):
    timestamp_ms: int = betterproto.int64_field(1)
    value: float = betterproto.double_field(2)


@dataclass(eq=False, repr=False)
class Exemplar(betterproto.Message):
    labels: List[Label] = betterproto.message_field(1)
    value: float = betterproto.double_field(2)
    timestamp_ms: int = betterproto.int64_field(3)


@dataclass(eq=False, repr=False)
class MetricMetadata(betterproto.Message):
    type: MetricMetadataMetricType = betterproto.enum_field(1)
    metric_family_name: str = betterproto.string_field(2)
    help: str = betterproto.string_field(4)
    unit: str = betterproto.string_field(5)


@dataclass(eq=False, repr=False)
class QueryRequest(betterproto.Message):
    tenants: List[str] = betterproto.string_field(1)
    query: str = betterproto.string_field(2)


@dataclass(eq=False, repr=False)
class QueryRangeRequest(betterproto.Message):
    tenants: List[str] = betterproto.string_field(1)
    query: str = betterproto.string_field(2)
    start: datetime = betterproto.message_field(3)
    end: datetime = betterproto.message_field(4)
    step: timedelta = betterproto.message_field(5)


@dataclass(eq=False, repr=False)
class QueryResponse(betterproto.Message):
    data: bytes = betterproto.bytes_field(2)


@dataclass(eq=False, repr=False)
class ConfigRequest(betterproto.Message):
    config_modes: List[str] = betterproto.string_field(1)


@dataclass(eq=False, repr=False)
class ConfigResponse(betterproto.Message):
    config_yaml: List[str] = betterproto.string_field(4)


@dataclass(eq=False, repr=False)
class LoadRuleRequest(betterproto.Message):
    cluster_id: str = betterproto.string_field(1)
    namespace: str = betterproto.string_field(2)
    yaml_content: bytes = betterproto.bytes_field(3)


@dataclass(eq=False, repr=False)
class DeleteRuleRequest(betterproto.Message):
    cluster_id: str = betterproto.string_field(1)
    namespace: str = betterproto.string_field(2)
    group_name: str = betterproto.string_field(3)


@dataclass(eq=False, repr=False)
class GetRuleRequest(betterproto.Message):
    cluster_id: str = betterproto.string_field(1)
    namespace: str = betterproto.string_field(2)
    group_name: str = betterproto.string_field(3)


@dataclass(eq=False, repr=False)
class ListRulesRequest(betterproto.Message):
    cluster_id: List[str] = betterproto.string_field(1)
    rule_type: List[str] = betterproto.string_field(2)
    health_filter: List[str] = betterproto.string_field(3)
    state_filter: List[str] = betterproto.string_field(4)
    rule_name_regexp: str = betterproto.string_field(5)
    group_name_regexp: str = betterproto.string_field(6)
    list_invalid: bool = betterproto.bool_field(7)
    request_all: bool = betterproto.bool_field(8)
    namespace_regexp: str = betterproto.string_field(9)


@dataclass(eq=False, repr=False)
class ListRulesResponse(betterproto.Message):
    """
    matches the return of cortex ruler api
    https://github.com/cortexproject/cortex/blob/c0e4545fd26f33ca5cc3323ee48e4c2ccd182b83/pkg/ruler/api.go#L215
    """
    status: str = betterproto.string_field(1)
    data: RuleGroups = betterproto.message_field(2)


@dataclass(eq=False, repr=False)
class RuleGroups(betterproto.Message):
    groups: List[RuleGroup] = betterproto.message_field(1)


@dataclass(eq=False, repr=False)
class RuleGroup(betterproto.Message):
    name: str = betterproto.string_field(1)
    file: str = betterproto.string_field(2)
    rules: List[Rule] = betterproto.message_field(3)
    interval: float = betterproto.double_field(4)
    last_evaluation: str = betterproto.string_field(5)
    """ Note : string instead of timestamp to preserve compatibility with native prometheus api return value """
    evaluation_time: float = betterproto.double_field(6)
    cluster_id: str = betterproto.string_field(7)
    """ opni specific field """


@dataclass(eq=False, repr=False)
class Rule(betterproto.Message):
    """ combination of alerting and recording rule (alerting rule is a superset of recording) """
    state: str = betterproto.string_field(1)
    name: str = betterproto.string_field(2)
    query: str = betterproto.string_field(3)
    duration: float = betterproto.double_field(4)
    labels: Dict[str, str] = betterproto.map_field(5, key_type=str, value_type=str)
    annotations: Dict[str, str] = betterproto.map_field(6, key_type=str, value_type=str)
    health: str = betterproto.string_field(7)
    alerts: List[Alert] = betterproto.message_field(8)
    last_error: str = betterproto.string_field(9)
    type: str = betterproto.string_field(10)
    last_evaluation: str = betterproto.string_field(11)
    """ Note : string instead of timestamp to preserve compatibility with native prometheus api return value """
    evaluation_time: float = betterproto.double_field(12)


@dataclass(eq=False, repr=False)
class Alert(betterproto.Message):
    labels: Dict[str, str] = betterproto.map_field(1, key_type=str, value_type=str)
    annotations: Dict[str, str] = betterproto.map_field(2, key_type=str, value_type=str)
    state: str = betterproto.string_field(3)
    active_at: str = betterproto.string_field(4)
    """ Note : string instead of timestamp to preserve compatibility with native prometheus api return value """
    value: str = betterproto.string_field(5)


class CortexAdminStub(betterproto.ServiceStub):
    """
    The CortexAdmin service provides authenticated endpoints for internal
    Cortex APIs.
    """
    async def all_user_stats(
        self,
        empty: betterproto_lib_google_protobuf.Empty,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> UserIDStatsList:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/AllUserStats",
            empty,
            UserIDStatsList,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def write_metrics(
        self,
        write_request: WriteRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> WriteResponse:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/WriteMetrics",
            write_request,
            WriteResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def query(
        self,
        query_request: QueryRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> QueryResponse:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/Query",
            query_request,
            QueryResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def query_range(
        self,
        query_range_request: QueryRangeRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> QueryResponse:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/QueryRange",
            query_range_request,
            QueryResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def get_rule(
        self,
        get_rule_request: GetRuleRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> QueryResponse:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/GetRule",
            get_rule_request,
            QueryResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def get_metric_metadata(
        self,
        metric_metadata_request: MetricMetadataRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> MetricMetadata:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/GetMetricMetadata",
            metric_metadata_request,
            MetricMetadata,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def list_rules(
        self,
        list_rules_request: ListRulesRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> ListRulesResponse:
        """ Heavy-handed API for diagnostics. """
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/ListRules",
            list_rules_request,
            ListRulesResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def load_rules(
        self,
        load_rule_request: LoadRuleRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> betterproto_lib_google_protobuf.Empty:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/LoadRules",
            load_rule_request,
            betterproto_lib_google_protobuf.Empty,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def delete_rule(
        self,
        delete_rule_request: DeleteRuleRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> betterproto_lib_google_protobuf.Empty:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/DeleteRule",
            delete_rule_request,
            betterproto_lib_google_protobuf.Empty,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def flush_blocks(
        self,
        empty: betterproto_lib_google_protobuf.Empty,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> betterproto_lib_google_protobuf.Empty:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/FlushBlocks",
            empty,
            betterproto_lib_google_protobuf.Empty,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def get_series_metrics(
        self,
        series_request: SeriesRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> SeriesInfoList:
        """ list all metrics """
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/GetSeriesMetrics",
            series_request,
            SeriesInfoList,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def get_metric_label_sets(
        self,
        label_request: LabelRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> MetricLabels:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/GetMetricLabelSets",
            label_request,
            MetricLabels,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def get_cortex_status(
        self,
        empty: betterproto_lib_google_protobuf.Empty,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> CortexStatus:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/GetCortexStatus",
            empty,
            CortexStatus,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def get_cortex_config(
        self,
        config_request: ConfigRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> ConfigResponse:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/GetCortexConfig",
            config_request,
            ConfigResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )
    async def extract_raw_series(
        self,
        matcher_request: MatcherRequest,
        timeout: Optional[float] = None,
        deadline: Optional[Deadline] = None,
        metadata: Optional[MetadataLike] = None,
    ) -> QueryResponse:
        return await self._unary_unary(
            "/cortexadmin.CortexAdmin/ExtractRawSeries",
            matcher_request,
            QueryResponse,
            timeout=timeout,
            deadline=deadline,
            metadata=metadata,
        )


class CortexAdminBase(ServiceBase):
    """
    The CortexAdmin service provides authenticated endpoints for internal
    Cortex APIs.
    """
    async def all_user_stats(self, empty: betterproto_lib_google_protobuf.Empty) -> UserIDStatsList:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def write_metrics(self, write_request: WriteRequest) -> WriteResponse:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def query(self, query_request: QueryRequest) -> QueryResponse:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def query_range(self, query_range_request: QueryRangeRequest) -> QueryResponse:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def get_rule(self, get_rule_request: GetRuleRequest) -> QueryResponse:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def get_metric_metadata(self, metric_metadata_request: MetricMetadataRequest) -> MetricMetadata:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def list_rules(self, list_rules_request: ListRulesRequest) -> ListRulesResponse:
        """ Heavy-handed API for diagnostics. """
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def load_rules(self, load_rule_request: LoadRuleRequest) -> betterproto_lib_google_protobuf.Empty:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def delete_rule(self, delete_rule_request: DeleteRuleRequest) -> betterproto_lib_google_protobuf.Empty:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def flush_blocks(self, empty: betterproto_lib_google_protobuf.Empty) -> betterproto_lib_google_protobuf.Empty:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def get_series_metrics(self, series_request: SeriesRequest) -> SeriesInfoList:
        """ list all metrics """
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def get_metric_label_sets(self, label_request: LabelRequest) -> MetricLabels:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def get_cortex_status(self, empty: betterproto_lib_google_protobuf.Empty) -> CortexStatus:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def get_cortex_config(self, config_request: ConfigRequest) -> ConfigResponse:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def extract_raw_series(self, matcher_request: MatcherRequest) -> QueryResponse:
        raise grpclib.GRPCError(grpclib.const.Status.UNIMPLEMENTED)
    
    async def __rpc_all_user_stats(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.all_user_stats(request)
        await stream.send_message(response)
    async def __rpc_write_metrics(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.write_metrics(request)
        await stream.send_message(response)
    async def __rpc_query(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.query(request)
        await stream.send_message(response)
    async def __rpc_query_range(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.query_range(request)
        await stream.send_message(response)
    async def __rpc_get_rule(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.get_rule(request)
        await stream.send_message(response)
    async def __rpc_get_metric_metadata(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.get_metric_metadata(request)
        await stream.send_message(response)
    async def __rpc_list_rules(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.list_rules(request)
        await stream.send_message(response)
    async def __rpc_load_rules(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.load_rules(request)
        await stream.send_message(response)
    async def __rpc_delete_rule(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.delete_rule(request)
        await stream.send_message(response)
    async def __rpc_flush_blocks(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.flush_blocks(request)
        await stream.send_message(response)
    async def __rpc_get_series_metrics(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.get_series_metrics(request)
        await stream.send_message(response)
    async def __rpc_get_metric_label_sets(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.get_metric_label_sets(request)
        await stream.send_message(response)
    async def __rpc_get_cortex_status(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.get_cortex_status(request)
        await stream.send_message(response)
    async def __rpc_get_cortex_config(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.get_cortex_config(request)
        await stream.send_message(response)
    async def __rpc_extract_raw_series(self, stream: grpclib.server.Stream) -> None:
        request = await stream.recv_message()
        response = await self.extract_raw_series(request)
        await stream.send_message(response)

    def __mapping__(self) -> Dict[str, grpclib.const.Handler]:
        return {
            "/cortexadmin.CortexAdmin/AllUserStats": grpclib.const.Handler(
                self.__rpc_all_user_stats,
                grpclib.const.Cardinality.UNARY_UNARY,
                betterproto_lib_google_protobuf.Empty,
                UserIDStatsList,
            ),
            "/cortexadmin.CortexAdmin/WriteMetrics": grpclib.const.Handler(
                self.__rpc_write_metrics,
                grpclib.const.Cardinality.UNARY_UNARY,
                WriteRequest,
                WriteResponse,
            ),
            "/cortexadmin.CortexAdmin/Query": grpclib.const.Handler(
                self.__rpc_query,
                grpclib.const.Cardinality.UNARY_UNARY,
                QueryRequest,
                QueryResponse,
            ),
            "/cortexadmin.CortexAdmin/QueryRange": grpclib.const.Handler(
                self.__rpc_query_range,
                grpclib.const.Cardinality.UNARY_UNARY,
                QueryRangeRequest,
                QueryResponse,
            ),
            "/cortexadmin.CortexAdmin/GetRule": grpclib.const.Handler(
                self.__rpc_get_rule,
                grpclib.const.Cardinality.UNARY_UNARY,
                GetRuleRequest,
                QueryResponse,
            ),
            "/cortexadmin.CortexAdmin/GetMetricMetadata": grpclib.const.Handler(
                self.__rpc_get_metric_metadata,
                grpclib.const.Cardinality.UNARY_UNARY,
                MetricMetadataRequest,
                MetricMetadata,
            ),
            "/cortexadmin.CortexAdmin/ListRules": grpclib.const.Handler(
                self.__rpc_list_rules,
                grpclib.const.Cardinality.UNARY_UNARY,
                ListRulesRequest,
                ListRulesResponse,
            ),
            "/cortexadmin.CortexAdmin/LoadRules": grpclib.const.Handler(
                self.__rpc_load_rules,
                grpclib.const.Cardinality.UNARY_UNARY,
                LoadRuleRequest,
                betterproto_lib_google_protobuf.Empty,
            ),
            "/cortexadmin.CortexAdmin/DeleteRule": grpclib.const.Handler(
                self.__rpc_delete_rule,
                grpclib.const.Cardinality.UNARY_UNARY,
                DeleteRuleRequest,
                betterproto_lib_google_protobuf.Empty,
            ),
            "/cortexadmin.CortexAdmin/FlushBlocks": grpclib.const.Handler(
                self.__rpc_flush_blocks,
                grpclib.const.Cardinality.UNARY_UNARY,
                betterproto_lib_google_protobuf.Empty,
                betterproto_lib_google_protobuf.Empty,
            ),
            "/cortexadmin.CortexAdmin/GetSeriesMetrics": grpclib.const.Handler(
                self.__rpc_get_series_metrics,
                grpclib.const.Cardinality.UNARY_UNARY,
                SeriesRequest,
                SeriesInfoList,
            ),
            "/cortexadmin.CortexAdmin/GetMetricLabelSets": grpclib.const.Handler(
                self.__rpc_get_metric_label_sets,
                grpclib.const.Cardinality.UNARY_UNARY,
                LabelRequest,
                MetricLabels,
            ),
            "/cortexadmin.CortexAdmin/GetCortexStatus": grpclib.const.Handler(
                self.__rpc_get_cortex_status,
                grpclib.const.Cardinality.UNARY_UNARY,
                betterproto_lib_google_protobuf.Empty,
                CortexStatus,
            ),
            "/cortexadmin.CortexAdmin/GetCortexConfig": grpclib.const.Handler(
                self.__rpc_get_cortex_config,
                grpclib.const.Cardinality.UNARY_UNARY,
                ConfigRequest,
                ConfigResponse,
            ),
            "/cortexadmin.CortexAdmin/ExtractRawSeries": grpclib.const.Handler(
                self.__rpc_extract_raw_series,
                grpclib.const.Cardinality.UNARY_UNARY,
                MatcherRequest,
                QueryResponse,
            ),
        }