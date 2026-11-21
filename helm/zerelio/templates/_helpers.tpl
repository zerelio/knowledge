{{/*
Expand the name of the chart.
*/}}
{{- define "zerelio.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
Truncates at 63 chars because some Kubernetes name fields are limited.
*/}}
{{- define "zerelio.fullname" -}}
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
Create chart label value: "<name>-<version>"
*/}}
{{- define "zerelio.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "zerelio.labels" -}}
helm.sh/chart: {{ include "zerelio.chart" . }}
{{ include "zerelio.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "zerelio.selectorLabels" -}}
app.kubernetes.io/name: {{ include "zerelio.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "zerelio.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "zerelio.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
PostgreSQL hostname — returns the Bitnami sub-chart service name when
postgresql.enabled=true, otherwise falls back to externalDatabase.host.
*/}}
{{- define "zerelio.databaseHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "zerelio.fullname" .) }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Build the DATABASE_URL DSN string (postgres://user:pass@host:port/db).
Used only when externalDatabase.existingSecret is empty.
*/}}
{{- define "zerelio.databaseURL" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "postgres://postgres:%s@%s:5432/%s?sslmode=disable"
    .Values.postgresql.auth.postgresPassword
    (include "zerelio.databaseHost" .)
    .Values.postgresql.auth.database }}
{{- else }}
{{- printf "postgres://%s:%s@%s:%d/%s?sslmode=disable"
    .Values.externalDatabase.user
    .Values.externalDatabase.password
    .Values.externalDatabase.host
    (.Values.externalDatabase.port | int)
    .Values.externalDatabase.database }}
{{- end }}
{{- end }}
