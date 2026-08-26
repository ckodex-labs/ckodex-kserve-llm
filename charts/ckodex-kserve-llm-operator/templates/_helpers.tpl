{{/*
Expand the name of the chart.
*/}}
{{- define "ckodex-kserve-llm-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Hugging Face initializer image follows appVersion unless explicitly overridden. */}}
{{- define "ckodex-kserve-llm-operator.huggingFaceInitializerImage" -}}
{{- default (printf "ghcr.io/ckodex-labs/ckodex-kserve-llm-huggingface-initializer:%s" .Chart.AppVersion) .Values.runtime.huggingFaceInitializerImage -}}
{{- end }}

{{/* ServiceAccount used by the manager. */}}
{{- define "ckodex-kserve-llm-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ckodex-kserve-llm-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* ServiceAccount used by the observe-only console. */}}
{{- define "ckodex-kserve-llm-operator.consoleServiceAccountName" -}}
{{- if .Values.console.serviceAccount.create }}
{{- default (printf "%s-console" (include "ckodex-kserve-llm-operator.fullname" .)) .Values.console.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.console.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "ckodex-kserve-llm-operator.fullname" -}}
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
{{- define "ckodex-kserve-llm-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "ckodex-kserve-llm-operator.labels" -}}
helm.sh/chart: {{ include "ckodex-kserve-llm-operator.chart" . }}
{{ include "ckodex-kserve-llm-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "ckodex-kserve-llm-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ckodex-kserve-llm-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
