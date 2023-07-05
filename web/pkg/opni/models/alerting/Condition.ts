import { Resource } from '../Resource';
import { cloneAlertCondition, deleteAlertCondition } from '../../utils/requests/alerts';
import { Duration, Reference, Status, Timestamp } from '../shared';
import { Cluster } from '../Cluster';

export enum Severity {
  INFO = 0, // eslint-disable-line no-unused-vars
  WARNING = 1, // eslint-disable-line no-unused-vars
  ERROR = 2, // eslint-disable-line no-unused-vars
  CRITICAL = 3, // eslint-disable-line no-unused-vars
}

export const SeverityResponseToEnum = {
  Info:     0,
  Warning:  1,
  Error:    2,
  Critical: 3,
};

export enum ControlFlowAction {
  IF_THEN = 0, // eslint-disable-line no-unused-vars
  IF_NOT_THEN = 1, // eslint-disable-line no-unused-vars
}

export enum CompositionAction {
  AND = 0, // eslint-disable-line no-unused-vars
  OR = 1, // eslint-disable-line no-unused-vars
}

export enum AlertType {
  SYSTEM = 0, // eslint-disable-line no-unused-vars
  KUBE_STATE = 1, // eslint-disable-line no-unused-vars
  COMPOSITION = 2, // eslint-disable-line no-unused-vars
  CONTROL_FLOW = 3, // eslint-disable-line no-unused-vars
  DOWNSTREAM_CAPABILTIY = 5, // eslint-disable-line no-unused-vars
  CPU = 5, // eslint-disable-line no-unused-vars
  MEMORY = 6, // eslint-disable-line no-unused-vars
  FS = 8, // eslint-disable-line no-unused-vars
  PROMETHEUS_QUERY = 9, // eslint-disable-line no-unused-vars
  MONITORING_BACKEND = 10, // eslint-disable-line no-unused-vars
}

export enum TimelineType {
  Timeline_Unknown = 0, // eslint-disable-line no-unused-vars, camelcase
  Timeline_Alerting = 1, // eslint-disable-line no-unused-vars, camelcase
  Timeline_Silenced = 2, // eslint-disable-line no-unused-vars, camelcase
}

export interface AlertConditionComposition {
  action: CompositionAction;
  x: Reference;
  y: Reference;
}

export interface AlertConditionControlFlow {
  action: ControlFlowAction;
  x: Reference;
  y: Reference;
  for: string;
}

export interface AlertConditionSystem {
  clusterId: Reference;
  timeout: Duration;
}

export interface AlertConditionKubeState {
  clusterId: string;
  objectType: string;
  objectName: string;
  namespace: string;
  state: string;
  for: Duration;
}

export interface AlertConditionPrometheusQuery {
  clusterId: Reference;
  query: string;
  for: Duration;
}

export interface AlertConditionDownstreamCapability {
  clusterId: Reference;
  capabilityState: string[];
  for: Duration;
}

export interface AlertConditionMonitoringBackend {
  backendComponents: string[];
  for: Duration;
}

export type Operation = '<' | '>' | '<=' | '>=' | '=' | '!=';

export interface Cores {
  items: Number[];
}

export interface AlertConditionCPUSaturation {
  clusterId: Reference;
  // optional filters for nodes and cores, restrict observation to said nodes or cores,
  // if empty, all nodes and cores are selected
  nodeCoreFilters: { [key: string]: Cores };
    // at least one cpu state should be specified
  cpuStates: string[];
  operation: Operation;
  expectedRatio: Number; // 0-1
  for: Duration;
}

export interface MemoryInfo {
  devices: string[];
}

export interface MemoryNodeGroup {
  nodes: { [key: string]: MemoryInfo };
}

export interface AlertConditionMemorySaturation {
  clusterId: Reference;
  nodeMemoryFilters: { [key: string]: MemoryInfo }; // nodes to devices
  // at least one usageType is required
  usageTypes: string[];
  operation: Operation;
  expectedRatio: Number;
  for: Duration;
}

export interface FilesystemInfo {
  mountpoints: string[];
  devices: string[];
}

export interface AlertConditionFilesystemSaturation {
  clusterId: Reference;
  // optional filters, if none are set then everything is selected
  nodeFilters: { [key: string]: FilesystemInfo};
  operation: Operation;
  expectedRatio: Number; // 0-1
  for: Duration;
}

export interface AlertTypeDetails {
    system: AlertConditionSystem;
    kubeState: AlertConditionKubeState;
    composition: AlertConditionComposition;
    controlFlow: AlertConditionControlFlow;
    downstreamCapability: AlertConditionDownstreamCapability;
    monitoringBackend: AlertConditionMonitoringBackend;
    cpu: AlertConditionCPUSaturation;
    memory: AlertConditionMemorySaturation;
    fs: AlertConditionFilesystemSaturation;
    prometheusQuery: AlertConditionPrometheusQuery;
}

export interface AttachedEndpoint {
  endpointId: string;
}

export interface EndpointImplementation {
  title: string;
  body: string;
  sendResolved: boolean;
}

export interface AttachedEndpoints {
  items: AttachedEndpoint[];

  initialDelay?: Duration;
  repeatInterval?: Duration;

  throttlingDuration?: Duration;
  details: EndpointImplementation;
}

export interface SilenceInfo {
  silenceId: string;
  startsAt: Timestamp;
  endsAt: Timestamp;
}

export interface AlertCondition {
  id: string;
  name: string;
  description: string;
  labels: string[];
  severity: Severity;
  alertType: AlertTypeDetails;
  attachedEndpoints: AttachedEndpoints;
  silence?: SilenceInfo;
}

export interface ActiveWindow {
  start: string;
  end: string;
  type: TimelineType;
}

export interface ActiveWindows {
  windows: ActiveWindow[];
}

export interface AlertConditionWithId {
  id: Reference;
  alertCondition: AlertCondition;
}

export interface AlertConditionList {
  items: AlertConditionWithId[];
}

export interface AlertDetailChoicesRequest {
  alertType: AlertType;
}

export interface ListAlertConditionSystem {
  agentIds: string[];
}

export interface ObjectList {
  objects: string;
}

export interface NamespaceObjects {
  namespaces: { [ key: string ]: ObjectList };
}

export interface KubeObjectGroups {
  resourceTypes: { [key: string]: NamespaceObjects };
}

export interface ListAlertConditionKubeState {
  clusters: { [key: string]: KubeObjectGroups };
  states: string[];
  fors: Duration;
}

export interface ListAlertConditionComposition {
  x: Reference;
  y: Reference;
}

export interface ListAlertConditionControlFlow {
  action: ControlFlowAction;
  x: Reference;
  y: Reference;
  for: Duration;
}

export interface ListAlertTypeDetails {
  system?: ListAlertConditionSystem;
  kubeState?: ListAlertConditionKubeState;
  composition?: ListAlertConditionComposition;
  controlFlow?: ListAlertConditionControlFlow;
}

export interface SilenceRequest {
  conditionId: Reference;
  duration: Duration;
}

export interface TimelineRequest {
  lookbackWindow: Duration;
}

export interface TimelineResponse {
  items: { [key: string]: ActiveWindows };
}

export interface UpdateAlertConditionRequest {
  id: Reference;
  updateAlert: AlertCondition;
}

export type StringStringPair = { [key: string]: string };
export interface MessageInstance {
  receivedAt: Timestamp;
  lastUpdatedAt: Timestamp;
  notification: Notification;
  startDetails: StringStringPair;
  lastDetails: StringStringPair;
}

export interface ListMessageResponse {
  items: MessageInstance[];
}

export interface ListAlarmMessageRequest {
  conditionId: string;
  fingerprints: string[];
  start: Timestamp;
  end: Timestamp;
}

export enum AlertConditionState {
  UNSPECIFIED = 0, // eslint-disable-line no-unused-vars, camelcase
  OK = 1, // eslint-disable-line no-unused-vars, camelcase
  PENDING = 2, // eslint-disable-line no-unused-vars, camelcase
  FIRING = 3, // eslint-disable-line no-unused-vars, camelcase
  SILENCED = 4, // eslint-disable-line no-unused-vars, camelcase
  INVALIDATED = 5, // eslint-disable-line no-unused-vars, camelcase

}

export interface AlertStatusResponse {
  state: AlertConditionState;
  reason?: string;
}

export interface AlertConditionWithStatus {
  alertCondition: AlertCondition;
  status: AlertStatusResponse;
}

export interface ListStatusResponse {
  alertConditions: { [key: string]: AlertConditionWithStatus };
}

const UPSTREAM_CLUSTER_ID = 'UPSTREAM_CLUSTER_ID';

export class Condition extends Resource {
  private base: AlertConditionWithStatus;
  private clusters;

  constructor(base: AlertConditionWithStatus, vue: any, clusters?: Cluster[]) {
    super(vue);
    this.base = base;
    this.clusters = clusters;
  }

  get nameDisplay() {
    return this.base.alertCondition.name;
  }

  get clusterId() {
    return this.alertType.clusterId?.id;
  }

  get clusterDisplay() {
    const clusterId = this.alertType.clusterId?.id || this.alertType.clusterId;

    if (clusterId === UPSTREAM_CLUSTER_ID) {
      return 'Upstream';
    }

    if (!clusterId || !this.clusters) {
      return 'Disconnected';
    }

    return this.clusters.find(c => c.id === clusterId)?.nameDisplay || 'Disconnected';
  }

  get description() {
    return this.base.alertCondition.description;
  }

  get id() {
    return this.base.alertCondition.id;
  }

  get type(): string {
    return Object.keys(this.base.alertCondition.alertType)[0];
  }

  get typeDisplay(): string {
    const mapping: any = {
      system:               'Agent Disconnect',
      kubeState:            'Kube State',
      downstreamCapability: 'Downstream Capability',
      monitoringBackend:    'Monitoring Backend',
      cpu:                  'CPU',
      memory:               'Memory',
      fs:                   'Filesystem',
      prometheusQuery:      'Prometheus'
    };

    return mapping[this.type] || 'Unknown';
  }

  get alertType(): any {
    return this.base.alertCondition.alertType[this.type as keyof AlertTypeDetails];
  }

  get labels(): string[] {
    return this.base?.alertCondition.labels || [];
  }

  get status(): Status {
    const mapping: { [key: number]: Status} = {
      [AlertConditionState.FIRING]: {
        message: 'Firing',
        state:   'error'
      },
      [AlertConditionState.OK]: {
        message: 'Ok',
        state:   'success'
      },
      [AlertConditionState.SILENCED]: {
        message: 'Silenced',
        state:   'warning'
      },
      [AlertConditionState.UNSPECIFIED]: {
        message: 'Unspecified',
        state:   'warning'
      },
      [AlertConditionState.INVALIDATED]: {
        message: 'Invalidated',
        state:   'error'
      },
      [AlertConditionState.PENDING]: {
        message: 'Pending',
        state:   'warning'
      },
    };

    const status = { ...mapping[this.base.status.state] || mapping[AlertConditionState.UNSPECIFIED] };

    status.longMessage = this.base.status.reason;

    return status;
  }

  get availableActions(): any[] {
    return [
      {
        action:    'edit',
        altAction: 'edit',
        label:     'Edit',
        icon:      'icon icon-edit',
        enabled:   true,
      },
      {
        action:    'cloneToClustersModal',
        altAction: 'clone',
        label:     'Clone',
        icon:      'icon icon-copy',
        enabled:   true,
      },
      {
        action:     'promptRemove',
        altAction:  'delete',
        label:      'Delete',
        icon:       'icon icon-trash',
        bulkable:   true,
        enabled:    true,
        bulkAction: 'promptRemove',
        weight:     -10, // Delete always goes last
      }
    ];
  }

  edit() {
    this.vue.$router.replace({
      name:   'alarm',
      params: { id: this.id }
    });
  }

  async remove() {
    await deleteAlertCondition(this.id);
    super.remove();
  }

  async clone(clusterIds: string[]) {
    await cloneAlertCondition(this.base.alertCondition, clusterIds);
  }

  cloneToClustersModal() {
    this.vue.$emit('clone', this);
  }
}
