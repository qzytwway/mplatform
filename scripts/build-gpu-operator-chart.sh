#!/bin/bash

# Builds MPlatform's customized GPU Operator chart: the upstream chart with
# the DCGM Exporter metrics collected by the cluster's Prometheus and the GPU
# dashboards of charts/addons/gpu-operator. See its README.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OVERLAY_DIR="$ROOT_DIR/charts/addons/gpu-operator"
REPOSITORY="https://helm.ngc.nvidia.com/nvidia"
CUSTOM_SUFFIX="mplatform.1"
DEFAULT_PROMETHEUS_LABELS='{release: kube-prometheus-stack}'

GPU_OPERATOR_VERSION="${GPU_OPERATOR_VERSION:-v26.7.2}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/chart-packages}"
PROMETHEUS_LABELS="${PROMETHEUS_LABELS:-$DEFAULT_PROMETHEUS_LABELS}"
VERSION="$GPU_OPERATOR_VERSION-$CUSTOM_SUFFIX"

fail() {
    echo "❌ $*" >&2
    exit 1
}

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT
CHART="$WORK_DIR/gpu-operator"

echo "📥 Pulling gpu-operator $GPU_OPERATOR_VERSION..."
helm pull gpu-operator --repo "$REPOSITORY" --version "$GPU_OPERATOR_VERSION" --untar --untardir "$WORK_DIR"

# Rewrites the single-line values of dcgmExporter.serviceMonitor; the checks
# below fail if the upstream layout changed.
echo "🛠️  Customizing values.yaml..."
awk -v labels="$PROMETHEUS_LABELS" '
    /^[^[:space:]#]/ { exporter = /^dcgmExporter:/; monitor = 0 }
    exporter && /^  [^[:space:]#]/ { monitor = /^  serviceMonitor:/ }
    monitor && /^    enabled:/ { print "    enabled: true"; next }
    monitor && /^    additionalLabels:/ { print "    additionalLabels: " labels; next }
    monitor && /^    relabelings:/ {
        print "    relabelings: [{sourceLabels: [__meta_kubernetes_pod_node_name], targetLabel: instance, action: replace}]"
        next
    }
    { print }
' "$CHART/values.yaml" > "$WORK_DIR/values.yaml"
mv "$WORK_DIR/values.yaml" "$CHART/values.yaml"

awk -v version="$VERSION" '/^version:/ { print "version: " version; next } { print }' \
    "$CHART/Chart.yaml" > "$WORK_DIR/Chart.yaml"
mv "$WORK_DIR/Chart.yaml" "$CHART/Chart.yaml"

cp -R "$OVERLAY_DIR/templates/." "$CHART/templates/"
mkdir -p "$CHART/dashboards"
cp "$OVERLAY_DIR"/dashboards/*.json "$CHART/dashboards/"

echo "🔍 Checking the customized chart..."
helm lint "$CHART" >/dev/null || fail "helm lint failed"
for dashboard in "$CHART"/dashboards/*.json; do
    python3 -c 'import json, sys; sys.exit(0 if json.load(open(sys.argv[1])).get("uid") else 1)' "$dashboard" \
        || fail "$(basename "$dashboard") is not a dashboard with a uid"
done

POLICY="$(helm template gpu-operator "$CHART" --namespace gpu-operator --show-only templates/clusterpolicy.yaml)"
MONITOR="$(awk '/^  dcgmExporter:/ { exporter = 1; next } exporter && /^  [^ ]/ { exit } exporter' <<< "$POLICY" \
    | awk '/^    serviceMonitor:/ { monitor = 1; next } monitor && /^    [^ ]/ { exit } monitor')"
grep -q "enabled: true" <<< "$MONITOR" || fail "the DCGM Exporter ServiceMonitor is not enabled"
grep -q "targetLabel: instance" <<< "$MONITOR" || fail "the DCGM Exporter ServiceMonitor keeps the pod instance"
LABEL_PAIRS="${PROMETHEUS_LABELS#\{}"
IFS=',' read -ra PAIRS <<< "${LABEL_PAIRS%\}}"
for pair in "${PAIRS[@]}"; do
    pair="$(xargs <<< "$pair")"
    grep -qF "$pair" <<< "$MONITOR" || fail "the DCGM Exporter ServiceMonitor lacks the label $pair"
done

DASHBOARDS="$(helm template gpu-operator "$CHART" --namespace gpu-operator --show-only templates/mplatform-dashboards.yaml)"
grep -q 'grafana_dashboard: "1"' <<< "$DASHBOARDS" || fail "the dashboards ConfigMap is not labeled grafana_dashboard"
for dashboard in "$CHART"/dashboards/*.json; do
    grep -qF "$(basename "$dashboard"):" <<< "$DASHBOARDS" || fail "the dashboards ConfigMap lacks $(basename "$dashboard")"
done

echo "📦 Packaging gpu-operator $VERSION..."
helm package "$CHART" --destination "$WORK_DIR" >/dev/null
mkdir -p "$OUT_DIR"
# The platform installs the newest version, and the -mplatform suffix sorts
# before the upstream release, so other gpu-operator packages would win.
for other in "$OUT_DIR"/gpu-operator-*.tgz; do
    [ -e "$other" ] || continue
    echo "🗑️  Removing $(basename "$other")"
    rm "$other"
done
mv "$WORK_DIR/gpu-operator-$VERSION.tgz" "$OUT_DIR/"
echo "✅ Built $OUT_DIR/gpu-operator-$VERSION.tgz"
