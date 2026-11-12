// Ready-made rules for what the built-in kube-prometheus-stack rules leave
// out: GPUs, inference services, and adjustable usage thresholds. Their title
// and description are translated under observability.alerts.templates.items.
export type TemplateCategory = 'gpu' | 'inference' | 'workload' | 'node'

export interface AlertRuleTemplate {
  name: string
  category: TemplateCategory
  expr: string
  for: string
  severity: 'critical' | 'warning' | 'info'
  // Prometheus template appended to the summary to name what fired.
  target: string
}

const INFERENCE_TARGET =
  '{{ $labels.namespace }}/{{ $labels.inference_service }}'

export const TEMPLATE_CATEGORIES: TemplateCategory[] = [
  'gpu',
  'inference',
  'workload',
  'node',
]

export const ALERT_RULE_TEMPLATES: AlertRuleTemplate[] = [
  {
    name: 'GPUTemperatureHigh',
    category: 'gpu',
    expr: 'max by (instance, gpu) (DCGM_FI_DEV_GPU_TEMP) > 85',
    for: '5m',
    severity: 'warning',
    target: '{{ $labels.instance }} GPU {{ $labels.gpu }}',
  },
  {
    name: 'GPUMemoryAlmostFull',
    category: 'gpu',
    expr: 'max by (instance, gpu) (DCGM_FI_DEV_FB_USED / (DCGM_FI_DEV_FB_USED + DCGM_FI_DEV_FB_FREE)) > 0.95',
    for: '10m',
    severity: 'warning',
    target: '{{ $labels.instance }} GPU {{ $labels.gpu }}',
  },
  {
    name: 'GPUXidError',
    category: 'gpu',
    expr: 'max by (instance, gpu) (DCGM_FI_DEV_XID_ERRORS) > 0',
    for: '1m',
    severity: 'critical',
    target: '{{ $labels.instance }} GPU {{ $labels.gpu }}',
  },
  {
    name: 'InferenceFirstTokenSlow',
    category: 'inference',
    expr: 'histogram_quantile(0.95, sum by (le, namespace, inference_service) (rate({__name__=~"vllm:time_to_first_token_seconds_bucket|sglang:time_to_first_token_seconds_bucket", inference_role!="prefill"}[5m]))) > 2',
    for: '10m',
    severity: 'warning',
    target: INFERENCE_TARGET,
  },
  {
    name: 'InferenceRequestsQueued',
    category: 'inference',
    expr: 'sum by (namespace, inference_service) ({__name__=~"vllm:num_requests_waiting|sglang:num_queue_reqs"}) > 10',
    for: '5m',
    severity: 'warning',
    target: INFERENCE_TARGET,
  },
  {
    name: 'InferenceKVCacheFull',
    category: 'inference',
    expr: 'max by (namespace, inference_service, pod) ({__name__=~"vllm:kv_cache_usage_perc|sglang:token_usage"}) > 0.95',
    for: '10m',
    severity: 'warning',
    target: '{{ $labels.namespace }}/{{ $labels.pod }}',
  },
  {
    name: 'PodFrequentRestarts',
    category: 'workload',
    expr: 'increase(kube_pod_container_status_restarts_total[15m]) > 3',
    for: '1m',
    severity: 'warning',
    target: '{{ $labels.namespace }}/{{ $labels.pod }}',
  },
  {
    name: 'ContainerOOMKilled',
    category: 'workload',
    expr: 'increase(kube_pod_container_status_restarts_total[10m]) > 0 and on (namespace, pod, container) kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1',
    for: '1m',
    severity: 'warning',
    target: '{{ $labels.namespace }}/{{ $labels.pod }} {{ $labels.container }}',
  },
  {
    name: 'ContainerMemoryNearLimit',
    category: 'workload',
    expr: 'max by (namespace, pod, container) (container_memory_working_set_bytes{container!=""}) / on (namespace, pod, container) max by (namespace, pod, container) (kube_pod_container_resource_limits{resource="memory"}) > 0.9',
    for: '5m',
    severity: 'warning',
    target: '{{ $labels.namespace }}/{{ $labels.pod }} {{ $labels.container }}',
  },
  {
    name: 'NodeCPUUsageHigh',
    category: 'node',
    expr: '1 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) > 0.9',
    for: '15m',
    severity: 'warning',
    target: '{{ $labels.instance }}',
  },
  {
    name: 'NodeMemoryUsageHigh',
    category: 'node',
    expr: '1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes > 0.9',
    for: '15m',
    severity: 'warning',
    target: '{{ $labels.instance }}',
  },
  {
    name: 'NodeDiskUsageHigh',
    category: 'node',
    expr: '1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs"} > 0.85',
    for: '10m',
    severity: 'warning',
    target: '{{ $labels.instance }} {{ $labels.mountpoint }}',
  },
]
