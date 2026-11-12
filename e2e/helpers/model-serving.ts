import {
  expect,
  type APIRequestContext,
  type APIResponse,
} from '@playwright/test'

import {
  crdInstalled,
  getObject,
  kubectl,
  kubectlJSON,
  minute,
  podReady,
  waitFor,
  type Container,
  type Pod,
} from './kube'

// Set by `make e2e-model-serving-run`, which builds these images and prepares
// the cluster these helpers expect: fake GPUs and RDMA NICs and the model
// directory.
const routerVersion = process.env.E2E_MS_ROUTER_VERSION || '0.3.2'
export const modelServing = {
  enabled: process.env.MPLATFORM_E2E_MODEL_SERVING === 'true',
  engineImage:
    process.env.E2E_MS_ENGINE_IMAGE || 'mplatform-e2e/fake-engine:0.1',
  routerImage:
    process.env.E2E_MS_ROUTER_IMAGE ||
    `mplatform-e2e/sglang-router:${routerVersion}`,
  vllmRouterImage:
    process.env.E2E_MS_VLLM_ROUTER_IMAGE || 'vllm/vllm-router:v0.1.17',
  modelDir: process.env.E2E_MS_MODEL_DIR || '/data/models/e2e-chat',
  namespace: process.env.E2E_MS_NAMESPACE || 'mplatform-ms-e2e',
}

export const serviceLabel = 'model-serving.mplatform.io/service'
export const roleLabel = 'model-serving.mplatform.io/role'
export const workerIndexLabel = 'leaderworkerset.sigs.k8s.io/worker-index'
export const bootstrapAnnotation = 'sglang.ai/bootstrap-port'
export const multusAnnotation = 'k8s.v1.cni.cncf.io/networks'

/** Lists the pods of a service, leaders first, by group. */
export function servicePods(namespace: string, name: string, extra = '') {
  const selector = [`${serviceLabel}=${name}`, extra].filter(Boolean).join(',')
  const pods = kubectlJSON<{ items: Pod[] }>([
    'get',
    'pods',
    '-n',
    namespace,
    '-l',
    selector,
  ]).items.filter((pod) => !pod.metadata.deletionTimestamp)
  return pods.sort((a, b) =>
    a.metadata.name.localeCompare(b.metadata.name, undefined, {
      numeric: true,
    })
  )
}

/** Explains what a service's pods are doing, for timeout messages. */
export function describeServicePods(namespace: string, name: string) {
  const pods = kubectl([
    'get',
    'pods',
    '-n',
    namespace,
    '-l',
    `${serviceLabel}=${name}`,
    '-o',
    'wide',
  ])
  const events = kubectl([
    'get',
    'events',
    '-n',
    namespace,
    '--sort-by=.lastTimestamp',
  ])
    .split('\n')
    .filter((line) => line.includes(name))
    .slice(-15)
    .join('\n')
  return `${pods}\n${events}`
}

// ---------------------------------------------------------------- platform API

export interface PDRole {
  replicas: number
  groupSize: number
  gpuCount: number
  extraArgs?: string[]
  env?: { name: string; value: string }[]
  nodeSelector?: Record<string, string>
  tolerations?: Toleration[]
}

export interface Toleration {
  key?: string
  operator?: 'Equal' | 'Exists'
  value?: string
  effect?: string
}

export interface ServiceRequest {
  name: string
  namespace: string
  modelId: number
  runtime: 'vllm' | 'sglang'
  image: string
  mode: 'single' | 'multi' | 'pd'
  replicas?: number
  groupSize?: number
  gpuCount?: number
  cpu?: string
  memory?: string
  nodeName?: string
  extraArgs?: string[]
  env?: { name: string; value: string }[]
  pd?: {
    prefill: PDRole
    decode: PDRole
    routerImage: string
    routerReplicas?: number
    routerPolicy?: string
    transferBackend?: 'mooncake' | 'mooncake-tcp' | 'nixl'
    ibDevices?: string
  }
  rdma?: {
    mode: 'device-plugin' | 'host'
    resource?: string
    resourceCount?: number
    networks?: string
  }
  nodeSelector?: Record<string, string>
  tolerations?: Toleration[]
  priorityClassName?: string
}

export interface ServiceRole {
  name: string
  replicas: number
  readyReplicas: number
  groupSize?: number
  gpuCount?: number
}

export interface InferenceService {
  name: string
  namespace: string
  model: string
  runtime: string
  mode: string
  image: string
  gpuCount: number
  groupSize: number
  roles: ServiceRole[]
  ready: boolean
  sharedSubdomain?: boolean
  editable?: boolean
  endpoint: string
  pods?: {
    name: string
    role: string
    groupIndex: number
    node: string
    ready: boolean
  }[]
}

export interface LWSStatus {
  lws: boolean
  disaggregatedSet: boolean
  installed: boolean
  chart: { name: string; version?: string; image?: string }
}

export async function expectJSON<T>(response: APIResponse, status: number) {
  const text = await response.text()
  expect(response.status(), text).toBe(status)
  return JSON.parse(text) as T
}

/** Calls the admin model serving API of one cluster. */
export class ModelServingAPI {
  constructor(
    readonly request: APIRequestContext,
    readonly cluster: string
  ) {}

  private get clusterPath() {
    return `/api/v1/admin/model-serving/clusters/${encodeURIComponent(this.cluster)}`
  }

  private servicePath(namespace: string, name: string) {
    return `${this.clusterPath}/services/${namespace}/${name}`
  }

  async status() {
    return expectJSON<LWSStatus>(
      await this.request.get(`${this.clusterPath}/status`),
      200
    )
  }

  installLWS() {
    return this.request.post(`${this.clusterPath}/lws`)
  }

  uninstallLWS() {
    return this.request.delete(`${this.clusterPath}/lws`)
  }

  /** Returns the id of the model, registering it when it is missing. */
  async ensureModel(model: {
    name: string
    type: 'chat' | 'embedding'
    sourceType: 'local' | 's3'
    path: string
  }) {
    const { items } = await expectJSON<{
      items: { id: number; name: string }[]
    }>(await this.request.get('/api/v1/admin/model-serving/models'), 200)
    const existing = items.find((item) => item.name === model.name)
    if (existing) {
      return existing.id
    }
    const created = await expectJSON<{ id: number }>(
      await this.request.post('/api/v1/admin/model-serving/models', {
        data: model,
      }),
      201
    )
    return created.id
  }

  createService(body: ServiceRequest) {
    return this.request.post(`${this.clusterPath}/services`, { data: body })
  }

  getService(namespace: string, name: string) {
    return this.request.get(this.servicePath(namespace, name))
  }

  async listServices() {
    const { items } = await expectJSON<{ items: InferenceService[] }>(
      await this.request.get(`${this.clusterPath}/services`),
      200
    )
    return items
  }

  scaleService(
    namespace: string,
    name: string,
    body: {
      replicas?: number
      prefillReplicas?: number
      decodeReplicas?: number
      routerReplicas?: number
    }
  ) {
    return this.request.put(`${this.servicePath(namespace, name)}/scale`, {
      data: body,
    })
  }

  /** The request the edit form starts from, with the current replicas. */
  async getServiceRequest(namespace: string, name: string) {
    return expectJSON<ServiceRequest>(
      await this.request.get(`${this.servicePath(namespace, name)}/request`),
      200
    )
  }

  updateService(namespace: string, name: string, body: ServiceRequest) {
    return this.request.put(this.servicePath(namespace, name), { data: body })
  }

  deleteService(namespace: string, name: string) {
    return this.request.delete(this.servicePath(namespace, name))
  }
}

/**
 * Creates a service, retrying only while the LWS webhooks start serving.
 * Returns the response so callers can assert on a rejection.
 */
export async function tryCreateService(
  api: ModelServingAPI,
  body: ServiceRequest
) {
  const deadline = Date.now() + minute
  for (;;) {
    const response = await api.createService(body)
    const text = await response.text()
    if (!text.includes('failed calling webhook') || Date.now() > deadline) {
      return { status: response.status(), text }
    }
    await new Promise((resolve) => setTimeout(resolve, 2_000))
  }
}

export async function createService(
  api: ModelServingAPI,
  body: ServiceRequest
) {
  const { status, text } = await tryCreateService(api, body)
  expect(status, text).toBe(201)
}

/** Lists the warnings Kubernetes recorded for a workload, such as failed creations. */
export function workloadWarnings(namespace: string, name: string) {
  return kubectlJSON<{ items: { reason: string; message: string }[] }>([
    'get',
    'events',
    '-n',
    namespace,
    '--field-selector',
    `involvedObject.name=${name},type=Warning`,
  ]).items.map((event) => `${event.reason}: ${event.message}`)
}

/**
 * Waits until a service has the given number of pods whose containers run,
 * or with ready set, pass their readiness probes.
 */
export function waitForPods(
  namespace: string,
  name: string,
  {
    pods,
    ready,
    timeout = 4 * minute,
  }: { pods: number; ready: boolean; timeout?: number }
) {
  return waitFor(
    `${namespace}/${name} to have ${pods} ${ready ? 'ready' : 'running'} pods`,
    () => {
      const items = servicePods(namespace, name)
      const matching = items.filter((pod) =>
        ready ? podReady(pod) : pod.status.phase === 'Running'
      )
      return items.length === pods && matching.length === pods
        ? items
        : undefined
    },
    { timeout, diagnose: () => describeServicePods(namespace, name) }
  )
}

/**
 * Waits for a service's pods to be ready, then checks that the platform
 * reports the service ready too. The latter is a soft assertion so a wrong
 * status does not hide what the cluster shows.
 */
export async function waitForServiceReady(
  api: ModelServingAPI,
  namespace: string,
  name: string,
  { pods, timeout = 4 * minute }: { pods: number; timeout?: number }
) {
  await waitForPods(namespace, name, { pods, ready: true, timeout })
  let service: InferenceService | undefined
  try {
    service = await waitFor(
      `the platform to report ${namespace}/${name} ready`,
      async () => {
        const current = await expectJSON<InferenceService>(
          await api.getService(namespace, name),
          200
        )
        service = current
        return current.ready ? current : undefined
      },
      { timeout: minute / 2 }
    )
  } catch {
    expect
      .soft(
        service?.ready,
        `platform status of ${name}: ${JSON.stringify(service?.roles)}`
      )
      .toBe(true)
  }
  return service
}

/** Creates a service and waits as waitForServiceReady does. */
export async function createReadyService(
  api: ModelServingAPI,
  body: ServiceRequest,
  { pods, timeout = 4 * minute }: { pods: number; timeout?: number }
) {
  await createService(api, body)
  return waitForServiceReady(api, body.namespace, body.name, { pods, timeout })
}

/**
 * Deletes a service through the platform and waits until nothing created for
 * it is left: the workload, the LeaderWorkerSets of pd roles, pods, Service
 * and the router with its RBAC objects.
 */
export async function deleteServiceAndWaitGone(
  api: ModelServingAPI,
  namespace: string,
  name: string
) {
  await expectJSON(await api.deleteService(namespace, name), 200)
  await waitFor(
    `${namespace}/${name} to be gone from the platform`,
    async () =>
      (await api.getService(namespace, name)).status() === 404
        ? true
        : undefined
  )
  await waitForNothingLeft(namespace, name)
}

function waitForNothingLeft(namespace: string, name: string) {
  return waitFor(
    `everything of ${namespace}/${name} to be deleted`,
    () => {
      const left = leftoverObjects(namespace, name)
      if (left.length === 0) {
        return true
      }
      throw new Error(`still present: ${left.join(', ')}`)
    },
    { timeout: 2 * minute }
  )
}

export const podMonitorCRD = 'podmonitors.monitoring.coreos.com'

export function leftoverObjects(namespace: string, name: string) {
  const kinds = [
    'pods',
    'services',
    'secrets',
    'deployments',
    'serviceaccounts',
    'roles',
    'rolebindings',
    'leaderworkersets',
    'disaggregatedsets',
    ...(crdInstalled(podMonitorCRD) ? [podMonitorCRD] : []),
  ]
  const byLabel = kubectl([
    'get',
    kinds.join(','),
    '-n',
    namespace,
    '-l',
    `${serviceLabel}=${name}`,
    '-o',
    'name',
  ])
  const children = kubectl([
    'get',
    'leaderworkersets',
    '-n',
    namespace,
    '-l',
    `disaggregatedset.x-k8s.io/name=${name}`,
    '-o',
    'name',
  ])
  const byName = ['leaderworkerset', 'disaggregatedset', 'service']
    .map((kind) => getObject(kind, namespace, name) && `${kind}/${name}`)
    .filter(Boolean) as string[]
  return [
    ...new Set([
      ...`${byLabel}\n${children}`.split('\n').filter(Boolean),
      ...byName,
    ]),
  ]
}

/**
 * Deletes every inference service of a namespace through the platform and
 * waits until nothing of them is left, for cleaning up after a test.
 */
export async function deleteNamespaceServices(
  api: ModelServingAPI,
  namespace: string
) {
  const names = (await api.listServices())
    .filter((item) => item.namespace === namespace)
    .map((item) => item.name)
  for (const name of names) {
    const response = await api.deleteService(namespace, name)
    if (response.status() !== 200 && response.status() !== 404) {
      throw new Error(
        `deleting ${namespace}/${name} failed with ${response.status()}: ${await response.text()}`
      )
    }
  }
  for (const name of names) {
    await waitForNothingLeft(namespace, name)
  }
}

// ---------------------------------------------------------------- fake engine

export interface EngineLaunch {
  argv: string[]
  env: Record<string, string>
  parsed: {
    runtime: string
    modelPath: string
    modelPathExists: boolean
    servedModelName: string
    host: string | null
    port: number | null
    tp: number
    nnodes: number
    nodeRank: number
    distInitAddr: string | null
    masterAddr: string | null
    disaggregationMode: string | null
    bootstrapPort: number | null
    transferBackend: string | null
  }
}

export interface EngineDist {
  role: 'leader' | 'worker'
  leader?: string
  resolved?: string | null
  connected?: boolean
  error?: string | null
  peer?: string
}

function taggedLines<T>(namespace: string, pod: string, tag: string) {
  return kubectl(['logs', '-n', namespace, pod])
    .split('\n')
    .filter((line) => line.startsWith(`${tag} `))
    .map((line) => JSON.parse(line.slice(tag.length + 1)) as T)
}

/** Reads the command line and environment the fake engine started with. */
export function engineLaunch(namespace: string, pod: string) {
  const [launch] = taggedLines<EngineLaunch>(
    namespace,
    pod,
    'FAKE_ENGINE_START'
  )
  if (!launch) {
    throw new Error(`${pod} logged no FAKE_ENGINE_START line`)
  }
  return launch
}

/** Waits for the fake engine of a pod that may just have started to log its launch. */
export function waitForEngineLaunch(namespace: string, pod: string) {
  return waitFor(
    `${pod} to log its launch`,
    () => engineLaunch(namespace, pod),
    {
      timeout: minute,
    }
  )
}

/**
 * Waits for a worker of a multi-node group to reach its leader and returns
 * how it went; on timeout, the last failed attempt.
 */
export async function engineDist(
  namespace: string,
  pod: string,
  timeout = 90_000
) {
  let last: EngineDist = { role: 'worker', error: 'no FAKE_ENGINE_DIST line' }
  try {
    return await waitFor(
      `${pod} to reach its group leader`,
      () => {
        const lines = taggedLines<EngineDist>(
          namespace,
          pod,
          'FAKE_ENGINE_DIST'
        ).filter((line) => line.role === 'worker')
        last = lines.at(-1) ?? last
        return lines.find((line) => !line.error)
      },
      { timeout }
    )
  } catch {
    return last
  }
}

/** Lists the HTTP requests the fake engine served, by path. */
export function engineRequests(namespace: string, pod: string) {
  return taggedLines<{
    method: string
    path: string
    server: 'http' | 'bootstrap'
    body: Record<string, unknown> | null
  }>(namespace, pod, 'FAKE_ENGINE_REQUEST')
}

/** Lists how an SGLang decode server reached prefill bootstrap servers. */
export function engineBootstraps(namespace: string, pod: string) {
  return taggedLines<{
    host: string | null
    port: number | null
    room: string | null
    status: number | null
    error: string | null
  }>(namespace, pod, 'FAKE_ENGINE_BOOTSTRAP')
}

/**
 * Lists the KV side channel connections of a vLLM pd pair: decode servers log
 * where they connected, prefill servers who connected to them.
 */
export function engineKVTransfers(namespace: string, pod: string) {
  return taggedLines<{
    role?: 'prefill'
    peer?: string
    host?: string
    port?: number
    connected?: boolean
    error?: string | null
  }>(namespace, pod, 'FAKE_ENGINE_KV')
}

/** Returns the value of the last occurrence of an option, as argparse does. */
export function argValue(argv: string[], name: string) {
  const index = argv.lastIndexOf(name)
  return index >= 0 ? argv[index + 1] : undefined
}

// ---------------------------------------------------------------- services

/** GETs a path from a Service through the API server proxy. */
export function serviceGet(namespace: string, name: string, path: string) {
  return kubectl([
    'get',
    '--raw',
    `/api/v1/namespaces/${namespace}/services/${name}:http/proxy${path}`,
  ])
}

const postScript = `
import sys, urllib.request
request = urllib.request.Request(sys.argv[1], data=sys.argv[2].encode(), headers={"Content-Type": "application/json"})
print(urllib.request.urlopen(request, timeout=30).read().decode())
`

/**
 * POSTs a JSON body from inside a pod whose image has python3, such as the
 * router or a fake engine, so the request goes through cluster DNS and the
 * Service like an in-cluster client's would.
 */
export function postFromPod(
  namespace: string,
  pod: string,
  url: string,
  body: unknown
) {
  return kubectl([
    'exec',
    '-n',
    namespace,
    pod,
    '--',
    'python3',
    '-c',
    postScript,
    url,
    JSON.stringify(body),
  ])
}

export interface RouterWorker {
  url: string
  type: 'prefill' | 'decode' | 'regular'
  bootstrapPort: number | null
  healthy: boolean | undefined
}

/**
 * Reads the workers the router discovered, through the service's Service, or
 * from one router pod when given.
 */
export function routerWorkers(namespace: string, name: string, pod?: string) {
  const raw = pod
    ? kubectl([
        'get',
        '--raw',
        `/api/v1/namespaces/${namespace}/pods/${pod}:8000/proxy/workers`,
      ])
    : serviceGet(namespace, name, '/workers')
  const body = JSON.parse(raw) as {
    workers: Record<string, any>[]
  }
  return body.workers.map((worker): RouterWorker => {
    const type = JSON.stringify(worker.worker_type ?? '').toLowerCase()
    const bootstrapPort =
      worker.bootstrap_port ??
      worker.worker_type?.Prefill?.bootstrap_port ??
      worker.worker_type?.prefill?.bootstrap_port ??
      null
    return {
      url: worker.url,
      type: type.includes('prefill')
        ? 'prefill'
        : type.includes('decode')
          ? 'decode'
          : 'regular',
      bootstrapPort,
      healthy: worker.is_healthy,
    }
  })
}

/**
 * Waits until the router, or the given router pod, has discovered exactly
 * the given worker counts.
 */
export function waitForRouterWorkers(
  namespace: string,
  name: string,
  expected: { prefill: number; decode: number },
  { timeout = 2 * minute, pod }: { timeout?: number; pod?: string } = {}
) {
  return waitFor(
    `router ${pod ?? `of ${name}`} to discover ${expected.prefill} prefill and ${expected.decode} decode workers`,
    () => {
      const workers = routerWorkers(namespace, name, pod)
      const count = (type: RouterWorker['type']) =>
        workers.filter((worker) => worker.type === type).length
      if (
        count('prefill') === expected.prefill &&
        count('decode') === expected.decode &&
        workers.every((worker) => worker.healthy !== false)
      ) {
        return workers
      }
      throw new Error(`router workers: ${JSON.stringify(workers)}`)
    },
    { timeout, diagnose: () => describeServicePods(namespace, name) }
  )
}

export function podURL(pod: Pod, port = 8000) {
  return `http://${pod.status.podIP}:${port}`
}

export function envOf(container: Container) {
  return Object.fromEntries(
    (container.env ?? []).map((item) => [item.name, item.value ?? ''])
  )
}
