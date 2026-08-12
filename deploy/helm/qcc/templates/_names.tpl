{{/*
qcc.fullname
Returns the full name of the release, truncated to 63 chars.
*/}}
{{- define "qcc.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
qcc.chart
Returns the chart name and version label value.
*/}}
{{- define "qcc.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
qcc.serviceAccountName
The controller ServiceAccount: serviceAccount.name when set, else derived.
*/}}
{{- define "qcc.serviceAccountName" -}}
{{- if .Values.serviceAccount.name -}}
{{- .Values.serviceAccount.name -}}
{{- else -}}
{{- printf "%s-controller" (include "qcc.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
qcc.executorServiceName
The executor Service name the controller dials over gRPC.
*/}}
{{- define "qcc.executorServiceName" -}}
{{- printf "%s-executor" (include "qcc.fullname" .) -}}
{{- end -}}
