{{/*
Expand the name of the chart.
*/}}
{{- define "mplatform.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "mplatform.fullname" -}}
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
{{- define "mplatform.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "mplatform.labels" -}}
helm.sh/chart: {{ include "mplatform.chart" . }}
{{ include "mplatform.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "mplatform.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mplatform.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "mplatform.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "mplatform.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}


{{- define "mplatform.secret" -}}
{{- if .Values.secret.existingSecret }}
{{- .Values.secret.existingSecret }}
{{- else }}
{{- printf "%s-secret" (include "mplatform.fullname" .) }}
{{- end }}
{{- end }}

{{/*
"true" when the chart runs its own PostgreSQL: db.type is postgres and no
external database was given (db.dsn, or DB_DSN in an existing Secret).
*/}}
{{- define "mplatform.postgresql.enabled" -}}
{{- if and (eq .Values.db.type "postgres") (not .Values.db.dsn) .Values.secret.create (not .Values.secret.existingSecret) -}}
true
{{- end -}}
{{- end }}

{{- define "mplatform.postgresql.fullname" -}}
{{- printf "%s-postgresql" (include "mplatform.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "mplatform.postgresql.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mplatform.name" . }}-postgresql
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Refuses an upgrade that would move a release from SQLite to PostgreSQL:
the new database starts empty and its first visitor could claim the super
user. Data is not migrated by the chart.
*/}}
{{- define "mplatform.validateDatabase" -}}
{{- if and .Release.IsUpgrade (ne .Values.db.type "sqlite") (not .Values.db.migratedFromSqlite) }}
{{- $deployment := lookup "apps/v1" "Deployment" .Release.Namespace (include "mplatform.fullname" .) }}
{{- range (dig "spec" "template" "spec" "containers" list $deployment) }}
{{- range (.env | default list) }}
{{- if and (eq .name "DB_TYPE") (eq (.value | default "") "sqlite") }}
{{- fail "This release stores its data in SQLite. Set db.type=sqlite to keep it, or migrate the data to the new database first and then set db.migratedFromSqlite=true." }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- with .Values.db.postgresql }}
{{- if and .password (not (regexMatch "^[A-Za-z0-9._~-]+$" .password)) }}
{{- fail "db.postgresql.password may only contain letters, digits and . _ ~ -" }}
{{- end }}
{{- if not (regexMatch "^[a-z_][a-z0-9_]*$" .username) }}
{{- fail "db.postgresql.username must be lower-case letters, digits and _" }}
{{- end }}
{{- if not (regexMatch "^[a-z_][a-z0-9_]*$" .database) }}
{{- fail "db.postgresql.database must be lower-case letters, digits and _" }}
{{- end }}
{{- end }}
{{- end }}