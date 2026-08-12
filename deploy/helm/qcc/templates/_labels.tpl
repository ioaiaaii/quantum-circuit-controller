{{/*
qcc.labels
Standard Kubernetes labels.
Usage: include "qcc.labels" .
*/}}
{{- define "qcc.labels" -}}
helm.sh/chart: {{ include "qcc.chart" . }}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
qcc.selectorLabels
Minimal labels for use in selector/matchLabels.
Usage: include "qcc.selectorLabels" .
*/}}
{{- define "qcc.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
