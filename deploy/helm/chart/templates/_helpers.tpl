{{/*
Chart name, overridable with nameOverride.
*/}}
{{- define "qcc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified release name. Truncated to 63 chars for the DNS label limit.
*/}}
{{- define "qcc.fullname" -}}
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

{{- define "qcc.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Suffixed resource name, truncated so fullname+suffix stays within 63 chars.
  {{ include "qcc.resourceName" (dict "suffix" "executor" "context" $) }}
*/}}
{{- define "qcc.resourceName" -}}
{{- $fullname := include "qcc.fullname" .context }}
{{- $maxLen := sub 62 (len .suffix) | int }}
{{- if gt (len $fullname) $maxLen }}
{{- printf "%s-%s" (trunc $maxLen $fullname | trimSuffix "-") .suffix | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" $fullname .suffix | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Labels applied to every object in the chart.
*/}}
{{- define "qcc.labels" -}}
helm.sh/chart: {{ include "qcc.chart" . }}
{{ include "qcc.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/part-of: {{ include "qcc.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Immutable identity labels. Never add to these: a Deployment's selector cannot
be changed after creation, so anything here is permanent for existing installs.
*/}}
{{- define "qcc.selectorLabels" -}}
app.kubernetes.io/name: {{ include "qcc.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Per-component variants. Pass the component name and the root context:
  {{ include "qcc.componentLabels" (dict "component" "executor" "context" $) }}
*/}}
{{- define "qcc.componentLabels" -}}
{{ include "qcc.labels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "qcc.componentSelectorLabels" -}}
{{ include "qcc.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
ServiceAccount the controller runs as.
*/}}
{{- define "qcc.serviceAccountName" -}}
{{- if and (not (.Values.serviceAccount.enabled | default true)) .Values.serviceAccount.name }}
{{- .Values.serviceAccount.name }}
{{- else }}
{{- include "qcc.resourceName" (dict "suffix" "controller-manager" "context" .) }}
{{- end }}
{{- end }}

{{/*
Image reference. Honours a digest in the repository (repo@sha256:...) by
skipping the tag, so digest-pinned charts render correctly.
  {{ include "qcc.image" .Values.manager.image }}
*/}}
{{- define "qcc.image" -}}
{{- if contains "@" .repository }}
{{- .repository }}
{{- else }}
{{- printf "%s:%s" .repository (.tag | default .defaultTag) }}
{{- end }}
{{- end }}

{{/*
Address the controller dials for the executor's gRPC service.
*/}}
{{- define "qcc.executorAddr" -}}
{{- printf "%s:%d" (include "qcc.resourceName" (dict "suffix" "executor" "context" .)) (int .Values.executor.service.port) }}
{{- end }}
