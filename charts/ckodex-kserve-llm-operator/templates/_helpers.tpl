{{/*
Expand the name of the chart.
*/}}
{{- define "ckodex-kserve-llm-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
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
Create the name of the service account to use.
*/}}
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

{{/* Resolve release-owned images from the chart application version by default. */}}
{{- define "ckodex-kserve-llm-operator.imageTag" -}}
{{- default .Chart.AppVersion .Values.image.tag -}}
{{- end }}

{{- define "ckodex-kserve-llm-operator.consoleImageTag" -}}
{{- default .Chart.AppVersion .Values.console.image.tag -}}
{{- end }}

{{- define "ckodex-kserve-llm-operator.huggingFaceInitializerImage" -}}
{{- if .Values.vllm.huggingFaceInitializerImage -}}
{{- .Values.vllm.huggingFaceInitializerImage -}}
{{- else -}}
ghcr.io/ckodex-labs/ckodex-kserve-llm-huggingface-initializer:{{ .Chart.AppVersion }}
{{- end -}}
{{- end }}
