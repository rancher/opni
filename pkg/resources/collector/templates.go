package collector

import "html/template"

const (
	logReceiverK8s  = "filelog/k8s"
	logReceiverRKE  = "filelog/rke"
	logReceiverK3s  = "journald/k3s"
	logReceiverRKE2 = "journald/rke2"
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
    regex: '^.*\/(?P<namespace>[^_]+)_(?P<pod_name>[^_]+)_(?P<uid>[a-f0-9\-]+)\/(?P<container_name>[^\._]+)\/(?P<restart_count>\d+)\.log$'
    parse_from: attributes["log.file.path"]
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
journald/k8s:
  operators:
  # Filter in only related units
  - type: filter
    id: filter
    expr: >-
      ($$record._SYSTEMD_UNIT != "addon-config.service") &&
      ($$record._SYSTEMD_UNIT != "addon-run.service") &&
      ($$record._SYSTEMD_UNIT != "cfn-etcd-environment.service") &&
      ($$record._SYSTEMD_UNIT != "cfn-signal.service") &&
      ($$record._SYSTEMD_UNIT != "clean-ca-certificates.service") &&
      ($$record._SYSTEMD_UNIT != "containerd.service") &&
      ($$record._SYSTEMD_UNIT != "coreos-metadata.service") &&
      ($$record._SYSTEMD_UNIT != "coreos-setup-environment.service") &&
      ($$record._SYSTEMD_UNIT != "coreos-tmpfiles.service") &&
      ($$record._SYSTEMD_UNIT != "dbus.service") &&
      ($$record._SYSTEMD_UNIT != "docker.service") &&
      ($$record._SYSTEMD_UNIT != "efs.service") &&
      ($$record._SYSTEMD_UNIT != "etcd-member.service") &&
      ($$record._SYSTEMD_UNIT != "etcd.service") &&
      ($$record._SYSTEMD_UNIT != "etcd2.service") &&
      ($$record._SYSTEMD_UNIT != "etcd3.service") &&
      ($$record._SYSTEMD_UNIT != "etcdadm-check.service") &&
      ($$record._SYSTEMD_UNIT != "etcdadm-reconfigure.service") &&
      ($$record._SYSTEMD_UNIT != "etcdadm-save.service") &&
      ($$record._SYSTEMD_UNIT != "etcdadm-update-status.service") &&
      ($$record._SYSTEMD_UNIT != "flanneld.service") &&
      ($$record._SYSTEMD_UNIT != "format-etcd2-volume.service") &&
      ($$record._SYSTEMD_UNIT != "kube-node-taint-and-uncordon.service") &&
      ($$record._SYSTEMD_UNIT != "kubelet.service") &&
      ($$record._SYSTEMD_UNIT != "ldconfig.service") &&
      ($$record._SYSTEMD_UNIT != "locksmithd.service") &&
      ($$record._SYSTEMD_UNIT != "logrotate.service") &&
      ($$record._SYSTEMD_UNIT != "lvm2-monitor.service") &&
      ($$record._SYSTEMD_UNIT != "mdmon.service") &&
      ($$record._SYSTEMD_UNIT != "nfs-idmapd.service") &&
      ($$record._SYSTEMD_UNIT != "nfs-mountd.service") &&
      ($$record._SYSTEMD_UNIT != "nfs-server.service") &&
      ($$record._SYSTEMD_UNIT != "nfs-utils.service") &&
      ($$record._SYSTEMD_UNIT != "node-problem-detector.service") &&
      ($$record._SYSTEMD_UNIT != "ntp.service") &&
      ($$record._SYSTEMD_UNIT != "oem-cloudinit.service") &&
      ($$record._SYSTEMD_UNIT != "rkt-gc.service") &&
      ($$record._SYSTEMD_UNIT != "rkt-metadata.service") &&
      ($$record._SYSTEMD_UNIT != "rpc-idmapd.service") &&
      ($$record._SYSTEMD_UNIT != "rpc-mountd.service") &&
      ($$record._SYSTEMD_UNIT != "rpc-statd.service") &&
      ($$record._SYSTEMD_UNIT != "rpcbind.service") &&
      ($$record._SYSTEMD_UNIT != "set-aws-environment.service") &&
      ($$record._SYSTEMD_UNIT != "system-cloudinit.service") &&
      ($$record._SYSTEMD_UNIT != "systemd-timesyncd.service") &&
      ($$record._SYSTEMD_UNIT != "update-ca-certificates.service") &&
      ($$record._SYSTEMD_UNIT != "user-cloudinit.service") &&
      ($$record._SYSTEMD_UNIT != "var-lib-etcd2.service")
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
`))

	templateMainConfig = template.Must(template.New("main").Parse(`
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
        name: k8s.pod.uid
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
service:
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
`))
	templateAggregatorConfig = template.Must(template.New("aggregator").Parse(`
receivers:
  otlp:
    protocols:
      grpc: {}
      http: {}
processors:
  batch:
    send_batch_size: 1000
    timeout: 15s
  memory_limiter:
    limit_mib: 1000
    spike_limit_mib: 250
    check_interval: 1s
  attributes:
    actions:
    - key: cluster_id
      action: upsert
      value: {{ .ClusterID }}
exporters:
  otlphttp:
    endpoint: "{{ .AgentEndpoint }}"
    tls:
      insecure: true
service:
  pipelines:
  {{- if .LogsEnabled }}
    logs:
      receivers: ["otlp"]
      processors: ["memory_limiter"]
      exporters: ["otlphttp"]
  {{- end }}
`))
)

type AgentConfig struct {
	Instance string
	Logs     LoggingConfig
}

type LoggingConfig struct {
	Enabled   bool
	Receivers []string
}
type AggregatorConfig struct {
	AgentEndpoint string
	ClusterID     string
	LogsEnabled   bool
}
