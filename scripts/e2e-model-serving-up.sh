#!/usr/bin/env bash
# Prepares the kind cluster that model serving e2e runs against: two worker
# nodes that advertise fake GPUs and RDMA NICs, a model directory on every
# node, the fake engine and real SGLang router images, and the LWS chart in
# the local chart directory. Safe to rerun.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES="${ROOT_DIR}/e2e/fixtures/model-serving"
KIND_NAME="${E2E_MS_KIND_NAME:-mplatform-model-serving}"
KUBECONFIG="${E2E_MS_KUBECONFIG:-${TMPDIR:-/tmp/}mplatform-model-serving.kubeconfig}"
ENGINE_IMAGE="${E2E_MS_ENGINE_IMAGE:-mplatform-e2e/fake-engine:0.1}"
ROUTER_VERSION="${E2E_MS_ROUTER_VERSION:-0.3.2}"
ROUTER_IMAGE="${E2E_MS_ROUTER_IMAGE:-mplatform-e2e/sglang-router:${ROUTER_VERSION}}"
MODEL_DIR="${E2E_MS_MODEL_DIR:-/data/models/e2e-chat}"
GPUS_PER_NODE="${E2E_MS_GPUS_PER_NODE:-8}"
RDMA_PER_NODE="${E2E_MS_RDMA_PER_NODE:-8}"
LWS_CHART_VERSION="v1.0.0"
export KUBECONFIG

if kind get clusters | grep -qx "${KIND_NAME}"; then
  echo "☸️ Reusing kind cluster ${KIND_NAME}..."
  kind export kubeconfig --name "${KIND_NAME}" --kubeconfig "${KUBECONFIG}"
else
  echo "☸️ Creating kind cluster ${KIND_NAME}..."
  kind create cluster --name "${KIND_NAME}" --config "${FIXTURES}/kind-config.yaml" --wait 3m --kubeconfig "${KUBECONFIG}"
fi
kubectl wait --for=condition=Ready nodes --all --timeout=3m

# Extended resources patched into the node status stay until the node is
# recreated; the scheduler counts them like device plugin resources.
for node in $(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name); do
  echo "🎛️ Advertising ${GPUS_PER_NODE} nvidia.com/gpu and ${RDMA_PER_NODE} rdma/hca on ${node}..."
  for field in capacity allocatable; do
    kubectl patch "${node}" --subresource=status --type=json -p "[
      {\"op\":\"add\",\"path\":\"/status/${field}/nvidia.com~1gpu\",\"value\":\"${GPUS_PER_NODE}\"},
      {\"op\":\"add\",\"path\":\"/status/${field}/rdma~1hca\",\"value\":\"${RDMA_PER_NODE}\"}
    ]" >/dev/null
  done
done

for node in $(kind get nodes --name "${KIND_NAME}"); do
  echo "📁 Creating ${MODEL_DIR} on ${node}..."
  docker exec "${node}" sh -c "mkdir -p '${MODEL_DIR}' && echo '{\"model_type\":\"llama\"}' > '${MODEL_DIR}/config.json'"
done

echo "🐳 Building ${ENGINE_IMAGE} and ${ROUTER_IMAGE}..."
docker build --provenance=false -t "${ENGINE_IMAGE}" "${FIXTURES}/engine"
docker build --provenance=false --build-arg "SGLANG_ROUTER_VERSION=${ROUTER_VERSION}" -t "${ROUTER_IMAGE}" "${FIXTURES}/router"
kind load docker-image --name "${KIND_NAME}" "${ENGINE_IMAGE}" "${ROUTER_IMAGE}"

# The platform installs LWS from the local chart directory, which is not
# committed; the registry.k8s.io chart is the one model serving ships with.
if ! ls "${ROOT_DIR}"/chart-packages/lws-*.tgz >/dev/null 2>&1; then
  echo "📦 Pulling the LWS ${LWS_CHART_VERSION} chart into chart-packages..."
  mkdir -p "${ROOT_DIR}/chart-packages"
  pull_dir="$(mktemp -d)"
  helm pull oci://registry.k8s.io/lws/charts/lws --version "${LWS_CHART_VERSION}" -d "${pull_dir}"
  mv "${pull_dir}/lws-${LWS_CHART_VERSION}.tgz" "${ROOT_DIR}/chart-packages/lws-${LWS_CHART_VERSION#v}.tgz"
  rmdir "${pull_dir}"
fi

echo "✅ Model serving e2e cluster ${KIND_NAME} is ready (KUBECONFIG=${KUBECONFIG})"
