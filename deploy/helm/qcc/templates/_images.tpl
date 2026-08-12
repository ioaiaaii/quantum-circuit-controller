{{/*
qcc.controllerImage
Constructs the controller image endpoint, tag defaults to the chart appVersion.
*/}}
{{- define "qcc.controllerImage" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.controller.image.repository (.Values.controller.image.tag | default .Chart.AppVersion) -}}
{{- end -}}

{{/*
qcc.executorImage
Constructs the executor image endpoint, tag defaults to the chart appVersion.
*/}}
{{- define "qcc.executorImage" -}}
{{- printf "%s/%s:%s" .Values.image.registry .Values.executor.image.repository (.Values.executor.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
