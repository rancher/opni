package collector

import (
	"text/template"
)

const (
	logReceiverK8s      = "filelog/k8s"
	logReceiverRKE      = "filelog/rke"
	logReceiverK3s      = "journald/k3s"
	logReceiverRKE2     = "journald/rke2"
	fileLogReceiverRKE2 = "filelog/rke2"
)

var (
	// Receivers
	templateLogAgentK8sReceiver = `
filelog/k8s:
  include: [ /var/log/pods/*/*/*.log ]
  exclude: []
  start_at: beginning
  include_file_path: true
  include_file_name: false
  operators:
  # FInd out which format is used by kubernetes
  - type: router
    id: get-format
    routes:
    - output: parser-docker
      expr: 'body matches "^\\{"'
    - output: parser-crio
      expr: 'body matches "^[^ Z]+ "'
    - output: parser-containerd
      expr: 'body matches "^[^ Z]+Z"'
      # Parse CRI-O format
  - type: regex_parser
    id: parser-crio
    regex: '^(?P<time>[^ Z]+) (?P<stream>stdout|stderr) (?P<logtag>[^ ]*) ?(?P<log>.*)$'
    output: extract_metadata_from_filepath
    timestamp:
      parse_from: attributes.time
      layout_type: gotime
      layout: '2006-01-02T15:04:05.000000000-07:00'
    # Parse CRI-Containerd format
  - type: regex_parser
    id: parser-containerd
    regex: '^(?P<time>[^ ^Z]+Z) (?P<stream>stdout|stderr) (?P<logtag>[^ ]*) ?(?P<log>.*)$'
    output: extract_metadata_from_filepath
    timestamp:
      parse_from: attributes.time
      layout: '%Y-%m-%dT%H:%M:%S.%LZ'
    # Parse Docker format
  - type: json_parser
    id: parser-docker
    output: extract_metadata_from_filepath
    timestamp:
      parse_from: attributes.time
      layout: '%Y-%m-%dT%H:%M:%S.%LZ'
    # Extract metadata from file path
  - type: regex_parser
    id: extract_metadata_from_filepath
    regex: '^.*\/(?P<namespace>[^_]+)_(?P<pod_name>[^_]+)_((?P<confighash>[a-f0-9]{32})|(?P<uid>[0-9a-f]{8}\b-[0-9a-f]{4}\b-[0-9a-f]{4}\b-[0-9a-f]{4}\b-[0-9a-f]{12}))\/(?P<container_name>[^\._]+)\/(?P<restart_count>\d+)\.log$'
    parse_from: attributes["log.file.path"]
  - type: remove
    field: attributes["log.file.path"]
  # Move out attributes to Attributes
  - type: move
    id: move-namespace
    from: attributes.namespace
    to: resource["k8s.namespace.name"]
  - type: move
    id: move-pod-name
    from: attributes.pod_name
    to: resource["k8s.pod.name"]
  - type: move
    id: move-container-name
    from: attributes.container_name
    to: resource["k8s.container.name"]
  - type: move
    from: attributes.uid
    to: resource["k8s.pod.uid"]
  - type: move
    from: attributes.confighash
    to: resource["k8s.pod.confighash"]
`
	templateLogAgentRKE = `
filelog/rke:
  include: [ /var/lib/rancher/rke/log/*.log ]
  start_at: beginning
  include_file_path: true
  include_file_name: false
  operators:
  - type: json_parser
    id: parser-docker
    timestamp:
      parse_from: attributes.time
      layout: '%Y-%m-%dT%H:%M:%S.%LZ'
`
	templateLogAgentK3s = template.Must(template.New("k3sreceiver").Parse(`
journald/k3s:
  units: [ "k3s" ]
  directory: {{ . }}
`))
	templateLogAgentRKE2 = template.Must(template.New("rke2receiver").Parse(`
journald/rke2:
  units:
  - "rke2-server"
  - "rke2-agent"
  directory: {{ . }}
filelog/rke2:
  include: [ /var/lib/rancher/rke2/agent/logs/kubelet.log ]
  start_at: beginning
  include_file_path: true
  include_file_name: false
  operators:
  - type: regex_parser
    id: time-sev
    on_error: drop
    regex: '^(?P<klog_level>[IWEF])(?P<klog_time>\d{4} \d{2}:\d{2}:\d{2}\.\d+)'
    timestamp:
      parse_from: attributes.klog_time
      layout: '%m%d %H:%M:%S.%L'
    severity:
      parse_from: attributes.klog_level
      mapping:
        info: I
        warn: W
        error: E
        fatal: F
  - type: move
    from: body
    to: attributes.message
  - type: add
    field: attributes.log_type:
    value: controlplane
  - type: add
    field: attributes.kubernetes_component
    value: kubelet
`))

	templateMainConfig = `
receivers: ${file:/etc/otel/receivers.yaml}
exporters:
  otlp:
    endpoint: "{{ .Instance }}-otel-aggregator:4317"
    tls:
      insecure: true
    sending_queue:
      num_consumers: 4
      queue_size: 100
    retry_on_failure:
      enabled: true
processors:
  memory_limiter:
    limit_mib: 250
    spike_limit_mib: 50
    check_interval: 1s
  k8sattributes:
    passthrough: false
    pod_association:
    - sources:
      - from: resource_attribute
        name: k8s.pod.ip
    - sources:
      - from: resource_attribute
        name: k8s.pod.name
      - from: resource_attribute
        name: k8s.namespace.name
    - sources:
      - from: connection
    extract:
      metadata:
      - "k8s.deployment.name"
      - "k8s.statefulset.name"
      - "k8s.daemonset.name"
      - "k8s.cronjob.name"
      - "k8s.job.name"
      - "k8s.node.name"
      - "container.image.name"
      - "container.image.tag"
      labels:
      - key: tier
      - key: component
    {{ template "metrics-system-processor" . }}
service:
  telemetry:
    logs:
      level: {{ .LogLevel }}
    metrics:
      level: none
  pipelines:
  {{- if .Logs.Enabled }}
    logs:
      receivers:
      {{- range .Logs.Receivers }}
      - {{ . }}
      {{- end }}
      processors: ["memory_limiter", "k8sattributes"]
      exporters: ["otlp"]
  {{- end }}
  {{ template "metrics-node-pipeline" .}}
`

	templateAggregatorConfig = `
receivers:
  otlp:
    protocols:
      grpc: {}
      http: {}
  {{ template "metrics-prometheus-receiver" . }}
  {{ template "metrics-prometheus-discoverer" . }}
{{- if .LogsEnabled }}
  k8s_events:
    auth_type: serviceAccount
{{- end }}

processors:
  batch:
    send_batch_size: 1000
    timeout: 15s
  memory_limiter:
    limit_mib: 1000
    spike_limit_mib: 350 
    check_interval: 1s
  transform:
    log_statements:
    - context: log
      statements:
      - set(attributes["log_type"], "event") where attributes["k8s.event.uid"] != nil
  {{ template "metrics-prometheus-processor" .}}
exporters:
  otlphttp:
    endpoint: "{{ .AgentEndpoint }}"
    tls:
      insecure: true
    sending_queue:
      num_consumers: 4
      queue_size: 100
    retry_on_failure:
      enabled: true
  {{ template "metrics-remotewrite-exporter" .}}
service:
  telemetry:
    logs:
      level: {{ .LogLevel }}
    metrics:
      level: none
  pipelines:
  {{- if .LogsEnabled }}
    logs:
      receivers: ["otlp", "k8s_events"]
      processors: ["transform", "memory_limiter", "batch"]
      exporters: ["otlphttp"]
  {{- end }}
  {{ template "metrics-remotewrite-pipeline" .}}
`
)

func init() {
	// compile time validation
	template.Must(template.New("aggregator-config").Parse(templateAggregatorConfig))
	template.Must(template.New("main-config").Parse(templateMainConfig))
}
