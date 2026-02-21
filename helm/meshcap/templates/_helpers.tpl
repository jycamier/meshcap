{{/*
Chart name
*/}}
{{- define "meshcap.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fullname
*/}}
{{- define "meshcap.fullname" -}}
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
Common labels
*/}}
{{- define "meshcap.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "meshcap.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "meshcap.selectorLabels" -}}
app.kubernetes.io/name: {{ include "meshcap.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name
*/}}
{{- define "meshcap.serviceAccountName" -}}
{{- if .Values.aws.serviceAccount.name }}
{{- .Values.aws.serviceAccount.name }}
{{- else }}
{{- include "meshcap.fullname" . }}
{{- end }}
{{- end }}

{{/*
Buffer threshold (absolute value from percent)
*/}}
{{- define "meshcap.bufferThreshold" -}}
{{- div (mul .Values.autoscaling.bufferThresholdPercent .Values.collector.bufferChanSize) 100 }}
{{- end }}
