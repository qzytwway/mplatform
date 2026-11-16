# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Two groups share the console in roughly equal measure:

- **Platform / operations engineers** who run the Kubernetes clusters, GPUs, observability and alerting. They know Kubernetes well and use the console daily.
- **Algorithm / model engineers** who deploy and tune Inference Services themselves. They may not know Kubernetes well.

## Product Purpose

One console to operate several Kubernetes clusters and the large language models served on their GPUs: connect or create clusters, prepare GPUs, deploy vLLM / SGLang Inference Services, publish them as OpenAI-compatible APIs, and watch metrics, logs and alerts, all without writing YAML. Success means a user can tell at a glance whether anything is wrong, and get from a symptom to its cause and fix without leaving the console.

## Positioning

Kubernetes operations and model serving in one place: an Inference Service is traced through its Model, Engine, Replicas, pods, nodes and GPUs, alongside the metrics, logs and alerts for each, in the same console that manages the clusters themselves. Ships as a single binary or image.

## Operating Context

Three recurring situations, all on desktop browsers:

- **Incident triage:** an alert fires or a service slows down; the user must locate the problem fast.
- **Routine checks:** several times a day, confirm clusters and services are healthy.
- **Deploy and change:** add models, create or update Inference Services, scale, edit configuration.

Desktop only; no mobile layouts.

## Capabilities and Constraints

- Multi-cluster: kubeadm over SSH, kubeconfig upload, in-cluster, or Cluster Agent. Cluster and namespace scope (including all namespaces) apply across resource pages.
- Kubernetes resources and CRDs, YAML editing, scale/restart, Pod / node / kubectl terminals, global search, Helm app store.
- Model serving: Model registry, Inference Services in Single-node, Multi-node and PD disaggregation Deployment Modes, Higress gateway with per-Caller API keys and quotas, GPU (GPU Operator) and vGPU (HAMi) allocation, inference metrics.
- Observability: Prometheus metrics, VictoriaLogs logs, alert rules and notifications.
- Auth: local, OAuth, LDAP, MFA, Passkey; RBAC per cluster; audit log; API keys.
- Domain terms follow `GLOSSARY.md` (Model, Engine, Inference Service, Replica, Deployment Mode, ...).

## Brand Commitments

- Name: MPlatform. The existing logo (`ui/src/assets/`) stays.
- Light and dark themes must both remain fully usable.
- All user-visible text is bilingual (English and Simplified Chinese) via i18n.

## Evidence on Hand

- Screenshots in `docs/images/` (workbench, Prometheus metrics, logs).
- No customer names, testimonials or benchmarks exist; do not invent them.

## Product Principles

1. Problems first: what is broken right now outranks totals and inventory.
2. Status color is truth: color signals health and severity, never decoration.
3. Always show where you are: the current cluster and namespace scope is visible wherever it changes what you see.
4. Serve both audiences: Kubernetes depth for operators without making model engineers learn Kubernetes to deploy a model.
