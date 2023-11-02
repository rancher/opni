{{/*
Expand the name of the chart.
*/}}
{{- define "opni-agent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "opni-agent.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "opni-agent.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "opni-agent.labels" -}}
helm.sh/chart: {{ include "opni-agent.chart" . }}
{{ include "opni-agent.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "opni-agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "opni-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "opni-agent.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "opni-agent.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Provides the namespace the chart will be installed in using the builtin .Release.Namespace,
or, if provided, a manually overwritten namespace value.
*/}}
{{- define "opni-agent.namespace" -}}
{{- if .Values.namespaceOverride -}}
{{ .Values.namespaceOverride -}}
{{- else -}}
{{ .Release.Namespace }}
{{- end -}}
{{- end -}}

{{/*
Calculates the gateway address
*/}}
{{- define "opni-agent.gatewayAddress" -}}
{{- if .Values.bootstrapInCluster.enabled -}}
{{ .Values.address }}.{{ .Release.Namespace }}.svc:9090
{{- else -}}
{{ .Values.address }}
{{- end -}}
{{- end -}}

# Rancher
{{- define "system_default_registry" -}}
{{- if .Values.global.cattle.systemDefaultRegistry -}}
{{- printf "%s/" .Values.global.cattle.systemDefaultRegistry -}}
{{- end -}}
{{- end -}}