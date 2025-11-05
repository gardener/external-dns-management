{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "external-dns-management.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "external-dns-management.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "external-dns-management.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "priorityclassversion" -}}
{{- if semverCompare ">= 1.14-0" .Capabilities.KubeVersion.GitVersion -}}
scheduling.k8s.io/v1
{{- else if semverCompare ">= 1.11-0" .Capabilities.KubeVersion.GitVersion -}}
scheduling.k8s.io/v1beta1
{{- else -}}
scheduling.k8s.io/v1alpha1
{{- end -}}
{{- end -}}

{{- define "image" -}}
{{- if hasPrefix "sha256:" (required "$.tag is required" $.tag) -}}
{{ required "$.repository is required" $.repository }}@{{ required "$.tag is required" $.tag }}
{{- else -}}
{{ required "$.repository is required" $.repository }}:{{ required "$.tag is required" $.tag }}
{{- end -}}
{{- end -}}

{{- define "image-ng" -}}
{{- if hasPrefix "sha256:" (required "$.tag is required" $.tag) -}}
{{ required "$.repository is required" $.repository }}-next-generation@{{ required "$.tag is required" $.tag }}
{{- else -}}
{{ required "$.repository is required" $.repository }}-next-generation:{{ required "$.tag is required" $.tag }}
{{- end -}}
{{- end -}}
