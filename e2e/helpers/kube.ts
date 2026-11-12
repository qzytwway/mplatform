import { execFileSync } from 'node:child_process'
import { readdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { kindClusterName } from '../env'

// The model serving suite deletes namespaces, uninstalls LWS and removes
// CRDs, so every kubectl call is pinned to its own kind cluster.
const kindName = kindClusterName.replace(/^kind-/, '')
export const kubeContext = `kind-${kindName}`
// scripts/e2e-model-serving-up.sh puts this label on the cluster's nodes.
export const dedicatedNodeLabel = 'e2e.mplatform.io/model-serving'
// Marks what the suite created, so it only ever deletes its own objects.
export const ownerLabel = 'e2e.mplatform.io/installed-by'
const ownerValue = 'model-serving-e2e'
const namespacePrefix = 'mplatform-ms-e2e'

export const minute = 60_000
const defaultTimeout = minute

export interface KubeObject {
  metadata: {
    name: string
    namespace?: string
    uid?: string
    deletionTimestamp?: string
    labels?: Record<string, string>
    annotations?: Record<string, string>
    ownerReferences?: { kind: string; name: string }[]
  }
  spec?: any
  status?: any
}

export interface Container {
  name: string
  image: string
  command?: string[]
  args?: string[]
  env?: { name: string; value?: string }[]
  ports?: { containerPort: number }[]
  readinessProbe?: Probe
  startupProbe?: Probe
  livenessProbe?: Probe
  resources?: {
    requests?: Record<string, string>
    limits?: Record<string, string>
  }
  securityContext?: {
    privileged?: boolean
    capabilities?: { add?: string[] }
  }
}

export interface PodTemplate {
  metadata?: {
    labels?: Record<string, string>
    annotations?: Record<string, string>
  }
  spec: {
    containers: Container[]
    hostNetwork?: boolean
    dnsPolicy?: string
    serviceAccountName?: string
    nodeSelector?: Record<string, string>
    tolerations?: Record<string, unknown>[]
    priorityClassName?: string
  }
}

export interface Probe {
  httpGet?: { path: string; port: number | string }
  periodSeconds?: number
  timeoutSeconds?: number
  failureThreshold?: number
}

export interface Pod extends KubeObject {
  metadata: KubeObject['metadata'] & {
    labels: Record<string, string>
    annotations?: Record<string, string>
  }
  spec: PodTemplate['spec'] & { nodeName?: string; priority?: number }
  status: {
    phase: string
    podIP?: string
    hostIP?: string
    conditions?: { type: string; status: string }[]
    containerStatuses?: { name: string; restartCount: number }[]
  }
}

/** Runs kubectl against the suite's cluster and returns its output. */
export function kubectl(
  args: string[],
  { input, timeout = defaultTimeout }: { input?: string; timeout?: number } = {}
) {
  try {
    return execFileSync('kubectl', ['--context', kubeContext, ...args], {
      encoding: 'utf8',
      input,
      timeout,
      stdio: ['pipe', 'pipe', 'pipe'],
      maxBuffer: 64 * 1024 * 1024,
    })
  } catch (error) {
    const stderr = (error as { stderr?: string }).stderr ?? ''
    throw new Error(`kubectl ${args.join(' ')} failed: ${stderr || error}`)
  }
}

export function kubectlJSON<T>(args: string[]) {
  return JSON.parse(kubectl([...args, '-o', 'json'])) as T
}

/**
 * Returns the object, or undefined when it does not exist. Without a
 * namespace it looks up a cluster-scoped object.
 */
export function getObject<T = KubeObject>(
  kind: string,
  namespace: string | undefined,
  name: string
) {
  const scope = namespace ? ['-n', namespace] : []
  const output = kubectl([
    'get',
    kind,
    name,
    ...scope,
    '--ignore-not-found',
    '-o',
    'json',
  ])
  return output.trim() ? (JSON.parse(output) as T) : undefined
}

export function crdInstalled(name: string) {
  return !!getObject('crd', undefined, name)
}

export function podReady(pod: Pod) {
  return (
    pod.status.phase === 'Running' &&
    !!pod.status.conditions?.some(
      (condition) => condition.type === 'Ready' && condition.status === 'True'
    )
  )
}

/**
 * Polls check until it returns a value other than undefined. On timeout the
 * error carries the last failure, if the last poll failed, and what diagnose
 * reports.
 */
export async function waitFor<T>(
  description: string,
  check: () => T | undefined | Promise<T | undefined>,
  {
    timeout = 3 * minute,
    interval = 2_000,
    diagnose,
  }: { timeout?: number; interval?: number; diagnose?: () => string } = {}
): Promise<T> {
  const deadline = Date.now() + timeout
  let lastError = ''
  for (;;) {
    try {
      const value = await check()
      if (value !== undefined) {
        return value
      }
      lastError = ''
    } catch (error) {
      lastError = String(error)
    }
    if (Date.now() > deadline) {
      let details = ''
      try {
        details = diagnose?.() ?? ''
      } catch (error) {
        details = `diagnose failed: ${error}`
      }
      throw new Error(
        `Timed out after ${timeout / 1000}s waiting for ${description}` +
          (lastError ? `\nlast error: ${lastError}` : '') +
          (details ? `\n${details}` : '')
      )
    }
    await new Promise((resolve) => setTimeout(resolve, interval))
  }
}

// ---------------------------------------------------------------- guards

/**
 * Refuses to go on unless the context reaches a local kind cluster whose
 * nodes are the suite's: named after its kind cluster and labeled by the
 * setup script.
 */
export function assertDedicatedCluster() {
  const server = kubectl([
    'config',
    'view',
    '--minify',
    '-o',
    'jsonpath={.clusters[0].cluster.server}',
  ])
  const host = new URL(server).hostname
  if (!['127.0.0.1', 'localhost', '[::1]'].includes(host)) {
    throw new Error(
      `context ${kubeContext} reaches ${server}, not a local kind cluster`
    )
  }
  const escaped = kindName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const pattern = new RegExp(`^${escaped}-(control-plane|worker\\d*)$`)
  const nodes = kubectlJSON<{ items: KubeObject[] }>(['get', 'nodes']).items
  const foreign = nodes.filter(
    (node) =>
      !pattern.test(node.metadata.name) ||
      node.metadata.labels?.[dedicatedNodeLabel] !== 'true'
  )
  if (nodes.length === 0 || foreign.length > 0) {
    throw new Error(
      `context ${kubeContext} is not the model serving e2e cluster; unexpected nodes: ` +
        foreign.map((node) => node.metadata.name).join(', ')
    )
  }
}

function assertSuiteNamespace(namespace: string) {
  if (!namespace.startsWith(namespacePrefix)) {
    throw new Error(
      `namespace ${namespace} does not start with ${namespacePrefix}`
    )
  }
  const existing = getObject('namespace', undefined, namespace)
  if (existing && existing.metadata.labels?.[ownerLabel] !== ownerValue) {
    throw new Error(
      `namespace ${namespace} exists without ${ownerLabel}=${ownerValue}; ` +
        `if it is a leftover of this suite, delete it with ` +
        `kubectl --context ${kubeContext} delete namespace ${namespace}, ` +
        `or start over with make e2e-model-serving-down`
    )
  }
  return existing
}

function createSuiteNamespace(namespace: string) {
  kubectl(['apply', '-f', '-'], {
    input: JSON.stringify({
      apiVersion: 'v1',
      kind: 'Namespace',
      metadata: { name: namespace, labels: { [ownerLabel]: ownerValue } },
    }),
  })
}

/** Creates the suite's namespace unless it exists. */
export function ensureNamespace(namespace: string) {
  if (!assertSuiteNamespace(namespace)) {
    createSuiteNamespace(namespace)
  }
}

/** Deletes the suite's namespace with all it holds and creates it again. */
export function resetNamespace(namespace: string) {
  if (assertSuiteNamespace(namespace)) {
    kubectl(['delete', 'namespace', namespace, '--timeout=5m'], {
      timeout: 6 * minute,
    })
  }
  createSuiteNamespace(namespace)
}

// ---------------------------------------------------------------- CRDs

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')
const prometheusCRDs = {
  'podmonitors.monitoring.coreos.com': 'crd-podmonitors.yaml',
  'prometheuses.monitoring.coreos.com': 'crd-prometheuses.yaml',
}

/**
 * Installs the PodMonitor and Prometheus CRDs, without Prometheus Operator,
 * from the kube-prometheus-stack chart in the local chart directory that
 * scripts/e2e-model-serving-up.sh provides. CRDs it did not install are
 * never touched.
 */
export function installPrometheusCRDs() {
  const existing = Object.keys(prometheusCRDs).filter(crdInstalled)
  if (existing.length > 0) {
    throw new Error(
      `${existing.join(', ')} already exist; the suite only uses CRDs it installs. ` +
        `If they are leftovers of this suite, delete them with ` +
        `kubectl --context ${kubeContext} delete crd ${existing.join(' ')}`
    )
  }
  const charts = resolve(repoRoot, 'chart-packages')
  const chart = readdirSync(charts).find((file) =>
    /^kube-prometheus-stack-.*\.tgz$/.test(file)
  )
  if (!chart) {
    throw new Error(`no kube-prometheus-stack chart in ${charts}`)
  }
  const names = Object.keys(prometheusCRDs)
  try {
    for (const file of Object.values(prometheusCRDs)) {
      const crd = execFileSync(
        'tar',
        [
          '-xzOf',
          resolve(charts, chart),
          `kube-prometheus-stack/charts/crds/crds/${file}`,
        ],
        {
          encoding: 'utf8',
          timeout: defaultTimeout,
          maxBuffer: 64 * 1024 * 1024,
        }
      )
      // Server-side, since the Prometheus CRD is too large for the
      // annotation a client-side apply keeps.
      kubectl(['apply', '--server-side', '-f', '-'], { input: crd })
    }
    kubectl(['label', 'crd', ...names, `${ownerLabel}=${ownerValue}`])
    kubectl([
      'wait',
      '--for=condition=Established',
      ...names.map((name) => `crd/${name}`),
    ])
  } catch (error) {
    // Neither existed before, so whatever is there now is ours; left
    // unlabeled, it would block every later run.
    kubectl(['delete', 'crd', ...names, '--ignore-not-found'], {
      timeout: 3 * minute,
    })
    throw error
  }
}

/** Removes the CRDs installPrometheusCRDs installed, with their objects. */
export function removePrometheusCRDs() {
  kubectl(
    [
      'delete',
      'crd',
      '-l',
      `${ownerLabel}=${ownerValue}`,
      '--ignore-not-found',
    ],
    { timeout: 3 * minute }
  )
}

// ---------------------------------------------------------------- diagnostics

/** Collects what a namespace's pods did, to attach to a failed test. */
export function namespaceDiagnostics(namespace: string) {
  const collect = (run: () => string) => {
    try {
      return run()
    } catch (error) {
      return String(error)
    }
  }
  const items = [
    {
      name: 'pods-and-events',
      body: collect(() =>
        kubectl(['get', 'pods,events', '-n', namespace, '-o', 'wide'])
      ),
    },
  ]
  const pods = collect(() =>
    kubectl(['get', 'pods', '-n', namespace, '-o', 'name'])
  )
  for (const pod of pods
    .split('\n')
    .filter((line) => line.startsWith('pod/'))) {
    items.push({
      name: `logs-${pod.slice('pod/'.length)}`,
      body: collect(() =>
        kubectl([
          'logs',
          '-n',
          namespace,
          pod,
          '--all-containers',
          '--tail=300',
        ])
      ),
    })
  }
  return items
}
