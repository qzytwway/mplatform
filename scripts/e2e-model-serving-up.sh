#!/usr/bin/env bash
# Prepares the kind cluster that model serving e2e runs against: two worker
# nodes that advertise fake GPUs and RDMA NICs, a model directory on every
# node, the fake engine and real SGLang and vLLM router images, and the LWS
# and kube-prometheus-stack charts in the local chart directory. Safe to rerun.
# make e2e-model-serving-up runs it with every variable below set.
set -euo pipefail

for tool in kind kubectl helm docker; do
  command -v "${tool}" >/dev/null || { echo "❌ ${tool} is required" >&2; exit 1; }
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES="${ROOT_DIR}/e2e/fixtures/model-serving"
STATE_DIR="${E2E_MS_TMPDIR:-${ROOT_DIR}/e2e/.tmp}"
KIND_NAME="${E2E_MS_KIND_NAME:-mplatform-model-serving}"
KUBECONFIG="${E2E_MS_KUBECONFIG:-${STATE_DIR}/kubeconfig}"
ENGINE_IMAGE="${E2E_MS_ENGINE_IMAGE:-mplatform-e2e/fake-engine:0.1}"
ROUTER_VERSION="${E2E_MS_ROUTER_VERSION:-0.3.2}"
ROUTER_IMAGE="${E2E_MS_ROUTER_IMAGE:-mplatform-e2e/sglang-router:${ROUTER_VERSION}}"
VLLM_ROUTER_IMAGE="${E2E_MS_VLLM_ROUTER_IMAGE:-vllm/vllm-router:v0.1.17}"
MODEL_DIR="${E2E_MS_MODEL_DIR:-/data/models/e2e-chat}"
GPUS_PER_NODE="${E2E_MS_GPUS_PER_NODE:-8}"
RDMA_PER_NODE="${E2E_MS_RDMA_PER_NODE:-8}"
LWS_CHART_VERSION="v1.0.0"
PROMETHEUS_CHART_VERSION="93.1.1"
NODE_COUNT=3
export KUBECONFIG

mkdir -p "${STATE_DIR}"
WORK_DIR="$(mktemp -d "${STATE_DIR}/up.XXXXXX")"
trap 'rm -rf "${WORK_DIR}"' EXIT

# Captured first: with pipefail, grep -q closing the pipe early could fail it.
clusters="$(kind get clusters)"
if grep -qx "${KIND_NAME}" <<<"${clusters}"; then
  echo "☸️ Reusing kind cluster ${KIND_NAME}..."
  kind export kubeconfig --name "${KIND_NAME}" --kubeconfig "${KUBECONFIG}"
else
  echo "☸️ Creating kind cluster ${KIND_NAME}..."
  kind create cluster --name "${KIND_NAME}" --config "${FIXTURES}/kind-config.yaml" --wait 3m --kubeconfig "${KUBECONFIG}"
fi
nodes="$(kubectl get nodes -o name)"
if [ "$(wc -l <<<"${nodes}" | tr -d ' ')" -ne "${NODE_COUNT}" ]; then
  echo "❌ ${KIND_NAME} does not have ${NODE_COUNT} nodes; delete it with make e2e-model-serving-down" >&2
  exit 1
fi
kubectl wait --for=condition=Ready nodes --all --timeout=3m
# The suite refuses to run against nodes without this label.
kubectl label nodes --all e2e.mplatform.io/model-serving=true --overwrite >/dev/null

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

# vllm-router is a multi-platform image, which kind load docker-image cannot
# import from Docker's containerd image store; an archive of the node
# platform alone can.
docker image inspect "${VLLM_ROUTER_IMAGE}" >/dev/null 2>&1 || docker pull "${VLLM_ROUTER_IMAGE}"
docker save --platform "linux/$(docker version --format '{{.Server.Arch}}')" -o "${WORK_DIR}/vllm-router.tar" "${VLLM_ROUTER_IMAGE}"
kind load image-archive --name "${KIND_NAME}" "${WORK_DIR}/vllm-router.tar"

# The platform installs LWS from the local chart directory, which is not
# committed; the registry.k8s.io chart is the one model serving ships with.
# The PodMonitor scenario takes its CRDs from kube-prometheus-stack.
pull_chart() {
  local pattern="$1" ref="$2" version="$3" name="$4"
  ls "${ROOT_DIR}"/chart-packages/${pattern} >/dev/null 2>&1 && return
  echo "📦 Pulling ${ref}:${version} into chart-packages..."
  mkdir -p "${ROOT_DIR}/chart-packages" "${WORK_DIR}/${name}.d"
  helm pull "oci://${ref}" --version "${version}" -d "${WORK_DIR}/${name}.d"
  mv "${WORK_DIR}/${name}.d"/*.tgz "${ROOT_DIR}/chart-packages/${name}"
}
pull_chart 'lws-*.tgz' registry.k8s.io/lws/charts/lws "${LWS_CHART_VERSION}" "lws-${LWS_CHART_VERSION#v}.tgz"
pull_chart 'kube-prometheus-stack-*.tgz' ghcr.io/prometheus-community/charts/kube-prometheus-stack \
  "${PROMETHEUS_CHART_VERSION}" "kube-prometheus-stack-${PROMETHEUS_CHART_VERSION}.tgz"

echo "✅ Model serving e2e cluster ${KIND_NAME} is ready (KUBECONFIG=${KUBECONFIG})"
