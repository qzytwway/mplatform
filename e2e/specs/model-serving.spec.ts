import { expect, test } from '@playwright/test'

import { authFile } from '../env'
import {
  assertDedicatedCluster,
  crdInstalled,
  ensureNamespace,
  getObject,
  installPrometheusCRDs,
  kubeContext,
  kubectl,
  kubectlJSON,
  namespaceDiagnostics,
  removePrometheusCRDs,
  resetNamespace,
  waitFor,
  type Container,
  type KubeObject,
  type Pod,
  type PodTemplate,
} from '../helpers/kube'
import {
  argValue,
  bootstrapAnnotation,
  createReadyService,
  createService,
  deleteNamespaceServices,
  deleteServiceAndWaitGone,
  engineBootstraps,
  engineDist,
  engineKVTransfers,
  engineLaunch,
  engineRequests,
  envOf,
  expectJSON,
  modelServing,
  ModelServingAPI,
  multusAnnotation,
  podMonitorCRD,
  podURL,
  postFromPod,
  roleLabel,
  serviceGet,
  serviceLabel,
  servicePods,
  tryCreateService,
  waitForEngineLaunch,
  waitForPods,
  waitForRouterWorkers,
  waitForServiceReady,
  workerIndexLabel,
  workloadWarnings,
  type InferenceService,
  type ServiceRequest,
} from '../helpers/model-serving'

// The platform names a cluster imported from a kubeconfig after its context.
const clusterName = kubeContext
const installTitle =
  'installs LeaderWorkerSet with DisaggregatedSet from the local chart'
const { namespace } = modelServing
const modelName = 'e2e-chat'
const modelMount = `/models/${modelName}`
const managedBy = {
  'app.kubernetes.io/managed-by': 'mplatform-model-serving',
}
const sglangCommand = ['python3', '-m', 'sglang.launch_server']
const vllmCommand = ['vllm', 'serve']

let api: ModelServingAPI
let modelId: number

function service(
  overrides: Partial<ServiceRequest> & Pick<ServiceRequest, 'name' | 'mode'>
): ServiceRequest {
  return {
    namespace,
    modelId,
    runtime: 'sglang',
    image: modelServing.engineImage,
    replicas: 1,
    gpuCount: 1,
    ...overrides,
  }
}

function pdService(
  name: string,
  pd: Partial<NonNullable<ServiceRequest['pd']>> = {},
  overrides: Partial<ServiceRequest> = {}
): ServiceRequest {
  return service({
    name,
    mode: 'pd',
    pd: {
      prefill: { replicas: 1, groupSize: 1, gpuCount: 1 },
      decode: { replicas: 1, groupSize: 1, gpuCount: 1 },
      routerImage: modelServing.routerImage,
      ...pd,
    },
    rdma: { mode: 'device-plugin', resource: 'rdma/hca', resourceCount: 1 },
    ...overrides,
  })
}

function runtimeContainer(pod: Pod) {
  return pod.spec.containers[0]
}

function leaderOf(pods: Pod[]) {
  const leader = pods.find(
    (pod) => pod.metadata.labels[workerIndexLabel] === '0'
  )
  if (!leader) {
    throw new Error(`no leader among ${pods.map((pod) => pod.metadata.name)}`)
  }
  return leader
}

function workersOf(pods: Pod[]) {
  return pods.filter((pod) => pod.metadata.labels[workerIndexLabel] !== '0')
}

/** Asserts the pod of a group leader serves HTTP and a worker does not. */
function expectServingPorts(container: Container, serves: boolean) {
  if (serves) {
    expect
      .soft(container.ports)
      .toEqual([expect.objectContaining({ containerPort: 8000 })])
    expect.soft(container.readinessProbe).toBeTruthy()
    expect.soft(container.startupProbe).toBeTruthy()
  } else {
    expect.soft(container.ports).toBeUndefined()
    expect.soft(container.readinessProbe).toBeUndefined()
  }
}

/** Asserts that $(LWS_*) references were expanded into real values. */
async function expectExpandedGroupArgs(
  pod: Pod,
  runtime: 'sglang' | 'vllm',
  { size, rank }: { size: number; rank: number }
) {
  const { argv, env, parsed } = await waitForEngineLaunch(
    namespace,
    pod.metadata.name
  )
  expect.soft(argv.join(' ')).not.toContain('$(')
  expect.soft(env.LWS_GROUP_SIZE).toBe(String(size))
  expect.soft(env.LWS_WORKER_INDEX).toBe(String(rank))
  expect.soft(env.LWS_LEADER_ADDRESS).toBeTruthy()
  expect.soft(argValue(argv, '--nnodes')).toBe(String(size))
  expect.soft(argValue(argv, '--node-rank')).toBe(String(rank))
  if (runtime === 'sglang') {
    expect
      .soft(argValue(argv, '--dist-init-addr'))
      .toBe(`${env.LWS_LEADER_ADDRESS}:20000`)
  } else {
    expect.soft(argValue(argv, '--master-addr')).toBe(env.LWS_LEADER_ADDRESS)
  }
  // Only leaders serve the OpenAI API.
  expect.soft(argv.includes('--host')).toBe(rank === 0)
  expect.soft(argv.includes('--port')).toBe(rank === 0)
  expect.soft(parsed.modelPathExists).toBe(true)
  return { argv, env }
}

/** Asserts the Service routes to the group leaders and answers /v1/models. */
async function expectServiceServesModel(name: string, leaders: Pod[]) {
  const svc = getObject<KubeObject>('service', namespace, name)
  expect.soft(svc?.spec.selector).toEqual({
    ...managedBy,
    [serviceLabel]: name,
    [workerIndexLabel]: '0',
  })
  // EndpointSlices follow pod readiness a moment later.
  const want = leaders.map((pod) => pod.status.podIP).sort()
  await waitFor(`the endpoints of ${name} to be its leaders`, () => {
    const addresses = kubectlJSON<{
      items: { endpoints?: { addresses: string[] }[] }[]
    }>([
      'get',
      'endpointslices',
      '-n',
      namespace,
      '-l',
      `kubernetes.io/service-name=${name}`,
    ])
      .items.flatMap((slice) => slice.endpoints ?? [])
      .flatMap((endpoint) => endpoint.addresses)
      .sort()
    expect(addresses).toEqual(want)
    return true
  })
  const models = JSON.parse(serviceGet(namespace, name, '/v1/models'))
  expect
    .soft(models.data.map((item: { id: string }) => item.id))
    .toEqual([modelName])
}

/**
 * Asserts LeaderWorkerSet gives each group of a single or multi node service
 * a headless Service of its own, which also publishes not-ready leaders for
 * their workers to find.
 */
function expectHeadlessServicePerGroup(name: string) {
  const lws = getObject('leaderworkerset', namespace, name)
  expect.soft(lws?.spec.networkConfig?.subdomainPolicy).toBe('UniquePerReplica')
  const headless = getObject('service', namespace, `${name}-0`)
  expect.soft(headless?.spec.clusterIP).toBe('None')
  expect.soft(headless?.spec.publishNotReadyAddresses).toBe(true)
}

test.describe('model serving', () => {
  test.skip(
    !modelServing.enabled,
    'needs the cluster from make e2e-model-serving-run'
  )
  test.describe.configure({ timeout: 12 * 60_000 })

  test.beforeAll(async ({ playwright }, testInfo) => {
    if (process.env.MPLATFORM_E2E_BASE_URL) {
      throw new Error(
        'refusing to run against MPLATFORM_E2E_BASE_URL: the suite rewrites model storage and models of the platform it runs against'
      )
    }
    assertDedicatedCluster()
    ensureNamespace(namespace)
    const request = await playwright.request.newContext({
      baseURL: testInfo.project.use.baseURL,
      storageState: authFile,
    })
    api = new ModelServingAPI(request, clusterName)
    modelId = await api.ensureModel({
      name: modelName,
      type: 'chat',
      sourceType: 'local',
      path: modelServing.modelDir,
    })
  })

  // Every other scenario needs LWS; without it they are skipped rather than
  // failing one by one.
  test.beforeEach(async ({}, testInfo) => {
    if (testInfo.title === installTitle) {
      return
    }
    const status = await api.status()
    test.skip(
      !(status.installed && status.lws && status.disaggregatedSet),
      'LWS is not installed: the installation scenario failed'
    )
  })

  test.afterEach(async ({}, testInfo) => {
    if (testInfo.status !== testInfo.expectedStatus) {
      for (const { name, body } of namespaceDiagnostics(namespace)) {
        await testInfo.attach(name, { body, contentType: 'text/plain' })
      }
    }
    try {
      await deleteNamespaceServices(api, namespace)
    } finally {
      removePrometheusCRDs()
    }
  })

  test.afterAll(async () => {
    await api?.request.dispose()
  })

  test(installTitle, async () => {
    // Uninstalling refuses while services exist, so start from an empty
    // namespace; this also clears whatever a failed run left behind.
    resetNamespace(namespace)

    const before = await api.status()
    expect(before.chart).toEqual({
      name: 'lws',
      version: 'v1.0.0',
      image: 'registry.k8s.io/lws/lws:v1.0.0',
    })
    if (before.installed) {
      await expectJSON(await api.uninstallLWS(), 200)
      await waitFor('the LWS release to be removed', async () =>
        (await api.status()).installed ? undefined : true
      )
      await waitFor('the LWS controller to stop', () =>
        kubectl(['get', 'pods', '-n', 'lws-system', '-o', 'name']).trim()
          ? undefined
          : true
      )
    }

    await expectJSON(await api.installLWS(), 201)
    const again = await api.installLWS()
    expect(again.status(), await again.text()).toBe(409)

    await waitFor('the LWS controller to be available', () => {
      const deployment = getObject(
        'deployment',
        'lws-system',
        'lws-controller-manager'
      )
      return deployment?.status?.availableReplicas >= 1 ? true : undefined
    })
    expect(await api.status()).toMatchObject({
      lws: true,
      disaggregatedSet: true,
      installed: true,
    })
    // enableDisaggregatedSet adds the DisaggregatedSet validating webhook.
    const webhooks = kubectlJSON<{ items: { webhooks: { name: string }[] }[] }>(
      ['get', 'validatingwebhookconfigurations']
    )
      .items.flatMap((item) => item.webhooks)
      .map((webhook) => webhook.name)
    expect(webhooks.join(' ')).toContain('disaggregatedset')
  })

  test('single node vLLM and SGLang get tensor parallelism from gpuCount', async () => {
    const vllm = service({
      name: 'single-vllm',
      mode: 'single',
      runtime: 'vllm',
      gpuCount: 2,
      cpu: '100m',
      memory: '128Mi',
      extraArgs: ['--max-model-len', '4096'],
      env: [{ name: 'E2E_MARK', value: 'single-vllm' }],
    })
    const sglang = service({
      name: 'single-sglang',
      mode: 'single',
      gpuCount: 2,
    })
    await Promise.all([
      createReadyService(api, vllm, { pods: 1 }),
      createReadyService(api, sglang, { pods: 1 }),
    ])

    for (const req of [vllm, sglang]) {
      const lws = getObject('leaderworkerset', namespace, req.name)
      expect.soft(lws?.metadata.labels).toMatchObject({
        ...managedBy,
        [serviceLabel]: req.name,
      })
      expect.soft(lws?.metadata.annotations).toMatchObject({
        'model-serving.mplatform.io/mode': 'single',
        'model-serving.mplatform.io/runtime': req.runtime,
        'model-serving.mplatform.io/model': modelName,
      })
      expect.soft(lws?.spec.replicas).toBe(1)
      // LWS reconciles the group without errors and reports it ready.
      expect.soft(workloadWarnings(namespace, req.name)).toEqual([])
      expect
        .soft(lws?.status?.readyReplicas, 'LWS status.readyReplicas')
        .toBe(1)
      expect.soft(lws?.spec.leaderWorkerTemplate.size).toBe(1)
      expect.soft(lws?.spec.leaderWorkerTemplate.leaderTemplate).toBeUndefined()
      expect
        .soft(lws?.spec.rolloutStrategy.rollingUpdateConfiguration)
        .toMatchObject({
          maxSurge: 0,
          maxUnavailable: 1,
        })

      const pods = servicePods(namespace, req.name)
      expect(pods).toHaveLength(1)
      const container = runtimeContainer(pods[0])
      expect.soft(container.name).toBe(req.runtime)
      expect.soft(container.image).toBe(modelServing.engineImage)
      expect.soft(container.resources?.limits?.['nvidia.com/gpu']).toBe('2')
      expectServingPorts(container, true)

      const { argv, env, parsed } = engineLaunch(
        namespace,
        pods[0].metadata.name
      )
      expect.soft(parsed.modelPathExists).toBe(true)
      await expectServiceServesModel(req.name, pods)
      expectHeadlessServicePerGroup(req.name)
      if (req.runtime === 'vllm') {
        expect.soft(container.command).toEqual(vllmCommand)
        expect
          .soft(argv)
          .toEqual([
            modelMount,
            '--served-model-name',
            modelName,
            '--host',
            '0.0.0.0',
            '--port',
            '8000',
            '--tensor-parallel-size',
            '2',
            '--max-model-len',
            '4096',
          ])
        expect.soft(env.E2E_MARK).toBe('single-vllm')
        // Requests of extended resources default to their limits.
        expect.soft(container.resources?.requests).toEqual({
          cpu: '100m',
          memory: '128Mi',
          'nvidia.com/gpu': '2',
        })
        expect.soft(container.resources?.limits).toEqual({
          cpu: '100m',
          memory: '128Mi',
          'nvidia.com/gpu': '2',
        })
      } else {
        expect.soft(container.command).toEqual(sglangCommand)
        expect
          .soft(argv)
          .toEqual([
            '--model-path',
            modelMount,
            '--served-model-name',
            modelName,
            '--host',
            '0.0.0.0',
            '--port',
            '8000',
            '--enable-metrics',
            '--tp',
            '2',
          ])
      }

      const detail = await expectJSON<InferenceService>(
        await api.getService(namespace, req.name),
        200
      )
      expect.soft(detail).toMatchObject({
        mode: 'single',
        runtime: req.runtime,
        model: modelName,
        image: modelServing.engineImage,
        gpuCount: 2,
        groupSize: 1,
        ready: true,
        endpoint: `http://${req.name}.${namespace}.svc:8000/v1`,
        roles: [{ name: 'default', replicas: 1, readyReplicas: 1 }],
      })
      expect.soft(detail.pods?.map((pod) => pod.role)).toEqual(['leader'])
      // Only LeaderWorkerSets from before per-group Services share one.
      expect.soft(detail.sharedSubdomain).toBeUndefined()
    }

    // single-sglang-0 would take the name of the first group of single-sglang.
    const clash = await tryCreateService(
      api,
      service({ name: `${sglang.name}-0`, mode: 'single' })
    )
    expect.soft(clash.status, clash.text).toBe(409)

    await deleteServiceAndWaitGone(api, namespace, vllm.name)
    await deleteServiceAndWaitGone(api, namespace, sglang.name)
  })

  test('multi node groups expand LWS variables and recreate the group on restart', async () => {
    const sglang = service({
      name: 'multi-sglang',
      mode: 'multi',
      groupSize: 2,
    })
    const vllm = service({
      name: 'multi-vllm',
      mode: 'multi',
      runtime: 'vllm',
      groupSize: 2,
    })
    await Promise.all([createService(api, sglang), createService(api, vllm)])

    for (const req of [sglang, vllm]) {
      const lws = getObject('leaderworkerset', namespace, req.name)
      const template = lws?.spec.leaderWorkerTemplate
      expect.soft(template.size).toBe(2)
      expect.soft(template.restartPolicy).toBe('RecreateGroupOnPodRestart')
      const leaderTemplate = template.leaderTemplate as PodTemplate
      const workerTemplate = template.workerTemplate as PodTemplate
      expect.soft(leaderTemplate).toBeTruthy()
      expectServingPorts(leaderTemplate.spec.containers[0], true)
      expectServingPorts(workerTemplate.spec.containers[0], false)
      expect.soft(workloadWarnings(namespace, req.name)).toEqual([])
      expectHeadlessServicePerGroup(req.name)

      // Arguments are checked as soon as the pods run: a group whose nodes
      // cannot meet never gets ready.
      const pods = await waitForPods(namespace, req.name, {
        pods: 2,
        ready: false,
      })
      const leader = leaderOf(pods)
      const [worker] = workersOf(pods)
      expect(worker).toBeTruthy()
      const leaderRun = await expectExpandedGroupArgs(leader, req.runtime, {
        size: 2,
        rank: 0,
      })
      const workerRun = await expectExpandedGroupArgs(worker, req.runtime, {
        size: 2,
        rank: 1,
      })
      expect
        .soft(workerRun.env.LWS_LEADER_ADDRESS)
        .toBe(leaderRun.env.LWS_LEADER_ADDRESS)
      if (req.runtime === 'sglang') {
        expect.soft(argValue(leaderRun.argv, '--tp')).toBe('2')
      } else {
        expect
          .soft(argValue(leaderRun.argv, '--tensor-parallel-size'))
          .toBe('1')
        expect
          .soft(argValue(leaderRun.argv, '--pipeline-parallel-size'))
          .toBe('2')
        expect.soft(workerRun.argv).toContain('--headless')
        expect.soft(leaderRun.argv).not.toContain('--headless')
      }

      // The worker reaches its leader by the address LWS gave it.
      const dist = await engineDist(namespace, worker.metadata.name)
      expect.soft(dist.resolved, JSON.stringify(dist)).toBe(leader.status.podIP)
      if (req.runtime === 'sglang') {
        expect.soft(dist.connected, JSON.stringify(dist)).toBe(true)
      }

      await waitForServiceReady(api, namespace, req.name, { pods: 2 })
      await expectServiceServesModel(req.name, [leader])
      const detail = await expectJSON<InferenceService>(
        await api.getService(namespace, req.name),
        200
      )
      expect
        .soft(detail)
        .toMatchObject({ mode: 'multi', groupSize: 2, gpuCount: 1 })
      expect
        .soft(detail.pods?.map((pod) => pod.role))
        .toEqual(['leader', 'worker'])
    }

    // Losing a worker restarts the whole group, leader included.
    const before = leaderOf(servicePods(namespace, sglang.name))
    kubectl([
      'delete',
      'pod',
      '-n',
      namespace,
      workersOf(servicePods(namespace, sglang.name))[0].metadata.name,
      '--wait=false',
    ])
    await waitFor('the multi-sglang leader to be recreated', () => {
      const pods = servicePods(namespace, sglang.name)
      const leader = pods.find(
        (pod) => pod.metadata.labels[workerIndexLabel] === '0'
      )
      return leader && leader.metadata.uid !== before.metadata.uid
        ? true
        : undefined
    })
    await waitForServiceReady(api, namespace, sglang.name, { pods: 2 })

    await deleteServiceAndWaitGone(api, namespace, sglang.name)
    await deleteServiceAndWaitGone(api, namespace, vllm.name)
  })

  test('pd on single nodes is wired to the SGLang router', async () => {
    const req = pdService(
      'pd-basic',
      {
        prefill: {
          replicas: 1,
          groupSize: 1,
          gpuCount: 1,
          extraArgs: ['--chunked-prefill-size', '4096'],
        },
        decode: {
          replicas: 1,
          groupSize: 1,
          gpuCount: 1,
          extraArgs: ['--cuda-graph-max-bs', '64'],
        },
      },
      {
        extraArgs: ['--trust-remote-code'],
        env: [{ name: 'E2E_MARK', value: 'pd-basic' }],
      }
    )
    await createReadyService(api, req, { pods: 3 })

    const ds = getObject('disaggregatedset', namespace, req.name)
    expect
      .soft(ds?.metadata.labels)
      .toMatchObject({ ...managedBy, [serviceLabel]: req.name })
    expect
      .soft(ds?.spec.roles.map((role: { name: string }) => role.name))
      .toEqual(['prefill', 'decode'])
    const children = kubectlJSON<{ items: KubeObject[] }>([
      'get',
      'leaderworkersets',
      '-n',
      namespace,
      '-l',
      `disaggregatedset.x-k8s.io/name=${req.name}`,
    ]).items
    expect
      .soft(
        children
          .map(
            (child) => child.metadata.labels?.['disaggregatedset.x-k8s.io/role']
          )
          .sort()
      )
      .toEqual(['decode', 'prefill'])

    const prefill = servicePods(namespace, req.name, `${roleLabel}=prefill`)
    const decode = servicePods(namespace, req.name, `${roleLabel}=decode`)
    expect(prefill).toHaveLength(1)
    expect(decode).toHaveLength(1)
    expect
      .soft(prefill[0].metadata.annotations?.[bootstrapAnnotation])
      .toBe('8998')
    expect
      .soft(decode[0].metadata.annotations?.[bootstrapAnnotation])
      .toBeUndefined()

    const prefillRun = engineLaunch(namespace, prefill[0].metadata.name)
    const decodeRun = engineLaunch(namespace, decode[0].metadata.name)
    const common = [
      '--model-path',
      modelMount,
      '--served-model-name',
      modelName,
      '--host',
      '0.0.0.0',
      '--port',
      '8000',
      '--enable-metrics',
    ]
    expect
      .soft(prefillRun.argv)
      .toEqual([
        ...common,
        '--disaggregation-mode',
        'prefill',
        '--disaggregation-bootstrap-port',
        '8998',
        '--disaggregation-transfer-backend',
        'mooncake',
        '--trust-remote-code',
        '--chunked-prefill-size',
        '4096',
      ])
    expect
      .soft(decodeRun.argv)
      .toEqual([
        ...common,
        '--disaggregation-mode',
        'decode',
        '--disaggregation-transfer-backend',
        'mooncake',
        '--trust-remote-code',
        '--cuda-graph-max-bs',
        '64',
      ])
    for (const run of [prefillRun, decodeRun]) {
      expect.soft(run.env.E2E_MARK).toBe('pd-basic')
      expect.soft(run.env.MOONCAKE_PROTOCOL).toBeUndefined()
    }
    for (const pod of [...prefill, ...decode]) {
      expect
        .soft(runtimeContainer(pod).resources?.limits?.['nvidia.com/gpu'])
        .toBe('1')
    }

    // The router and the RBAC its pod discovery needs.
    const routerName = `${req.name}-router`
    const role = getObject<{ rules: unknown }>('role', namespace, routerName)
    expect.soft(role?.rules).toEqual([
      {
        apiGroups: [''],
        resources: ['pods'],
        verbs: ['get', 'list', 'watch'],
      },
    ])
    expect.soft(getObject('rolebinding', namespace, routerName)).toBeTruthy()
    expect.soft(getObject('serviceaccount', namespace, routerName)).toBeTruthy()
    const [router] = servicePods(namespace, req.name, `${roleLabel}=router`)
    expect.soft(router.spec.serviceAccountName).toBe(routerName)
    expect.soft(runtimeContainer(router).image).toBe(modelServing.routerImage)
    expect
      .soft(getObject('service', namespace, req.name)?.spec.selector)
      .toEqual({
        ...managedBy,
        [serviceLabel]: req.name,
        [roleLabel]: 'router',
      })

    const workers = await waitForRouterWorkers(namespace, req.name, {
      prefill: 1,
      decode: 1,
    })
    expect.soft(workers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: 'prefill',
          url: podURL(prefill[0]),
          bootstrapPort: 8998,
        }),
        expect.objectContaining({ type: 'decode', url: podURL(decode[0]) }),
      ])
    )
    const routerLog = kubectl(['logs', '-n', namespace, router.metadata.name])
    expect.soft(routerLog).not.toMatch(/forbidden/i)

    // A request through the Service reaches both roles with the bootstrap
    // address of the prefill server, and decode answers it.
    const reply = JSON.parse(
      postFromPod(
        namespace,
        router.metadata.name,
        `http://${req.name}.${namespace}.svc:8000/v1/chat/completions`,
        {
          model: modelName,
          messages: [{ role: 'user', content: 'hello' }],
          max_tokens: 4,
        }
      )
    )
    expect.soft(reply.choices?.[0]?.message?.content).toBe('fake decode reply')
    const prefillRequests = engineRequests(namespace, prefill[0].metadata.name)
    const prefillChat = prefillRequests.find(
      (item) => item.server === 'http' && item.path === '/v1/chat/completions'
    )
    expect.soft(prefillChat?.body).toMatchObject({
      bootstrap_host: prefill[0].status.podIP,
      bootstrap_port: 8998,
    })
    const bootstraps = engineBootstraps(namespace, decode[0].metadata.name)
    expect.soft(bootstraps).toEqual([
      expect.objectContaining({
        host: prefill[0].status.podIP,
        port: 8998,
        status: 200,
      }),
    ])
    expect
      .soft(
        prefillRequests
          .filter((item) => item.server === 'bootstrap')
          .map((item) => item.path)
      )
      .toEqual([`/route?bootstrap_room=${bootstraps[0]?.room}`])

    const detail = await expectJSON<InferenceService>(
      await api.getService(namespace, req.name),
      200
    )
    expect.soft(detail).toMatchObject({
      mode: 'pd',
      ready: true,
      roles: [
        {
          name: 'prefill',
          replicas: 1,
          readyReplicas: 1,
          groupSize: 1,
          gpuCount: 1,
        },
        {
          name: 'decode',
          replicas: 1,
          readyReplicas: 1,
          groupSize: 1,
          gpuCount: 1,
        },
        { name: 'router', replicas: 1, readyReplicas: 1 },
      ],
    })
    expect
      .soft(detail.pods?.map((pod) => pod.role).sort())
      .toEqual(['decode', 'prefill', 'router'])
    // The LeaderWorkerSets of the roles are not listed as services.
    const listed = (await api.listServices()).map((item) => item.name)
    expect.soft(listed).toEqual([req.name])

    await deleteServiceAndWaitGone(api, namespace, req.name)
  })

  test('pd with a multi node prefill exposes only its leader to the router', async () => {
    const req = pdService('pd-multinode', {
      prefill: { replicas: 1, groupSize: 2, gpuCount: 1 },
      decode: { replicas: 1, groupSize: 1, gpuCount: 1 },
    })
    await createReadyService(api, req, { pods: 4 })

    const children = kubectlJSON<{ items: KubeObject[] }>([
      'get',
      'leaderworkersets',
      '-n',
      namespace,
      '-l',
      `disaggregatedset.x-k8s.io/name=${req.name},disaggregatedset.x-k8s.io/role=prefill`,
    ]).items
    expect(children).toHaveLength(1)
    const template = children[0].spec.leaderWorkerTemplate
    expect.soft(template.size).toBe(2)
    expect.soft(template.restartPolicy).toBe('RecreateGroupOnPodRestart')
    expectServingPorts(
      (template.workerTemplate as PodTemplate).spec.containers[0],
      false
    )

    const prefill = servicePods(namespace, req.name, `${roleLabel}=prefill`)
    const leader = leaderOf(prefill)
    const [worker] = workersOf(prefill)
    expect(worker).toBeTruthy()
    expect.soft(worker.metadata.annotations?.[bootstrapAnnotation]).toBe('8998')
    const leaderRun = await expectExpandedGroupArgs(leader, 'sglang', {
      size: 2,
      rank: 0,
    })
    const workerRun = await expectExpandedGroupArgs(worker, 'sglang', {
      size: 2,
      rank: 1,
    })
    for (const run of [leaderRun, workerRun]) {
      expect.soft(argValue(run.argv, '--tp')).toBe('2')
      expect.soft(argValue(run.argv, '--disaggregation-mode')).toBe('prefill')
      expect
        .soft(argValue(run.argv, '--disaggregation-bootstrap-port'))
        .toBe('8998')
    }
    const dist = await engineDist(namespace, worker.metadata.name)
    expect.soft(dist.connected, JSON.stringify(dist)).toBe(true)

    const [decode] = servicePods(namespace, req.name, `${roleLabel}=decode`)
    const decodeRun = engineLaunch(namespace, decode.metadata.name)
    expect.soft(decodeRun.argv).not.toContain('--dist-init-addr')
    expect.soft(decodeRun.argv).not.toContain('--tp')

    const workers = await waitForRouterWorkers(namespace, req.name, {
      prefill: 1,
      decode: 1,
    })
    const prefillURLs = workers
      .filter((item) => item.type === 'prefill')
      .map((item) => item.url)
    expect.soft(prefillURLs).toEqual([podURL(leader)])
    expect.soft(prefillURLs).not.toContain(podURL(worker))

    const detail = await expectJSON<InferenceService>(
      await api.getService(namespace, req.name),
      200
    )
    expect.soft(detail.roles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          name: 'prefill',
          groupSize: 2,
          gpuCount: 1,
          readyReplicas: 1,
        }),
        expect.objectContaining({
          name: 'decode',
          groupSize: 1,
          gpuCount: 1,
          readyReplicas: 1,
        }),
      ])
    )
    expect
      .soft(detail.pods?.map((pod) => pod.role).sort())
      .toEqual(['decode', 'prefill', 'prefill-worker', 'router'])

    await deleteServiceAndWaitGone(api, namespace, req.name)
  })

  test('rdma through a device plugin reaches every pd and multi node pod', async () => {
    const rdma = {
      mode: 'device-plugin' as const,
      resource: 'rdma/hca',
      resourceCount: 1,
      networks: 'default/rdma-net',
    }
    const pd = pdService(
      'pd-rdma',
      { transferBackend: 'mooncake', ibDevices: 'mlx5_0,mlx5_1' },
      { rdma }
    )
    const multi = service({
      name: 'multi-rdma',
      mode: 'multi',
      groupSize: 2,
      rdma: { ...rdma, resourceCount: 2, networks: 'rdma-a,rdma-b' },
    })
    // A single node needs no RDMA, so the platform drops it.
    const single = service({ name: 'single-rdma', mode: 'single', rdma })
    // Only the pod specs matter for the multi node and single services.
    await Promise.all([
      createReadyService(api, pd, { pods: 3 }),
      createService(api, multi).then(() =>
        waitForPods(namespace, multi.name, { pods: 2, ready: false })
      ),
      createService(api, single).then(() =>
        waitForPods(namespace, single.name, { pods: 1, ready: false })
      ),
    ])

    const expectRDMA = (pod: Pod, count: string, networks: string) => {
      const container = runtimeContainer(pod)
      expect
        .soft(container.resources?.limits?.['rdma/hca'], pod.metadata.name)
        .toBe(count)
      expect
        .soft(container.securityContext?.capabilities?.add, pod.metadata.name)
        .toEqual(['IPC_LOCK'])
      expect
        .soft(container.securityContext?.privileged, pod.metadata.name)
        .toBeFalsy()
      expect
        .soft(pod.metadata.annotations?.[multusAnnotation], pod.metadata.name)
        .toBe(networks)
      expect.soft(pod.spec.hostNetwork, pod.metadata.name).toBeFalsy()
    }
    const roles = [
      ...servicePods(namespace, pd.name, `${roleLabel}=prefill`),
      ...servicePods(namespace, pd.name, `${roleLabel}=decode`),
    ]
    expect(roles).toHaveLength(2)
    for (const pod of roles) {
      expectRDMA(pod, '1', 'default/rdma-net')
      const { argv } = engineLaunch(namespace, pod.metadata.name)
      expect
        .soft(argValue(argv, '--disaggregation-ib-device'))
        .toBe('mlx5_0,mlx5_1')
      expect
        .soft(argValue(argv, '--disaggregation-transfer-backend'))
        .toBe('mooncake')
    }
    const [prefillPod] = roles
    expect
      .soft(prefillPod.metadata.annotations?.[bootstrapAnnotation])
      .toBe('8998')
    const [router] = servicePods(namespace, pd.name, `${roleLabel}=router`)
    expect
      .soft(runtimeContainer(router).resources?.limits?.['rdma/hca'])
      .toBeUndefined()
    expect.soft(router.metadata.annotations?.[multusAnnotation]).toBeUndefined()
    await waitForRouterWorkers(namespace, pd.name, { prefill: 1, decode: 1 })

    for (const pod of servicePods(namespace, multi.name)) {
      expectRDMA(pod, '2', 'rdma-a,rdma-b')
    }

    const [singlePod] = servicePods(namespace, single.name)
    expect
      .soft(runtimeContainer(singlePod).resources?.limits?.['rdma/hca'])
      .toBeUndefined()
    expect.soft(runtimeContainer(singlePod).securityContext).toBeUndefined()
    expect
      .soft(singlePod.metadata.annotations?.[multusAnnotation])
      .toBeUndefined()

    for (const name of [pd.name, multi.name, single.name]) {
      await deleteServiceAndWaitGone(api, namespace, name)
    }
  })

  test('rdma in host mode runs groups in the node network with cluster DNS', async () => {
    const multi = service({
      name: 'multi-host',
      mode: 'multi',
      groupSize: 2,
      rdma: { mode: 'host' },
    })
    const pd = pdService('pd-host', {}, { rdma: { mode: 'host' } })
    // Leaders in the node network all bind port 8000, so one service at a
    // time fits the two worker nodes.
    await createService(api, multi)
    const pods = await waitForPods(namespace, multi.name, {
      pods: 2,
      ready: false,
    })
    for (const pod of pods) {
      expect.soft(pod.spec.hostNetwork, pod.metadata.name).toBe(true)
      expect
        .soft(pod.spec.dnsPolicy, pod.metadata.name)
        .toBe('ClusterFirstWithHostNet')
      expect
        .soft(
          runtimeContainer(pod).securityContext?.privileged,
          pod.metadata.name
        )
        .toBe(true)
      expect
        .soft(runtimeContainer(pod).resources?.limits?.['rdma/hca'])
        .toBeUndefined()
      expect.soft(pod.status.podIP).toBe(pod.status.hostIP)
    }
    const leader = leaderOf(pods)
    const [worker] = workersOf(pods)
    const dist = await engineDist(namespace, worker.metadata.name)
    expect.soft(dist.resolved, JSON.stringify(dist)).toBe(leader.status.podIP)
    expect.soft(dist.connected, JSON.stringify(dist)).toBe(true)
    await waitForServiceReady(api, namespace, multi.name, { pods: 2 })
    await expectServiceServesModel(multi.name, [leader])
    await deleteServiceAndWaitGone(api, namespace, multi.name)

    await createReadyService(api, pd, { pods: 3 })
    const prefill = servicePods(namespace, pd.name, `${roleLabel}=prefill`)
    const decode = servicePods(namespace, pd.name, `${roleLabel}=decode`)
    for (const pod of [...prefill, ...decode]) {
      expect.soft(pod.spec.hostNetwork, pod.metadata.name).toBe(true)
      expect
        .soft(pod.spec.dnsPolicy, pod.metadata.name)
        .toBe('ClusterFirstWithHostNet')
      expect
        .soft(
          runtimeContainer(pod).securityContext?.privileged,
          pod.metadata.name
        )
        .toBe(true)
    }
    const [router] = servicePods(namespace, pd.name, `${roleLabel}=router`)
    expect.soft(router.spec.hostNetwork).toBeFalsy()
    const workers = await waitForRouterWorkers(namespace, pd.name, {
      prefill: 1,
      decode: 1,
    })
    expect
      .soft(workers.map((item) => item.url).sort())
      .toEqual([podURL(prefill[0]), podURL(decode[0])].sort())
    await deleteServiceAndWaitGone(api, namespace, pd.name)
  })

  test('kv transfer backends set the mooncake tcp and nixl options', async () => {
    const tcp = pdService(
      'pd-tcp',
      { transferBackend: 'mooncake-tcp', ibDevices: 'mlx5_0' },
      { env: [{ name: 'E2E_MARK', value: 'pd-tcp' }] }
    )
    const nixl = pdService('pd-nixl', { transferBackend: 'nixl' })
    // The platform puts its env before the user's so the user's wins.
    const override = pdService(
      'pd-tcp-override',
      { transferBackend: 'mooncake-tcp' },
      { env: [{ name: 'MC_FORCE_TCP', value: '0' }] }
    )
    await Promise.all([
      createReadyService(api, tcp, { pods: 3 }),
      createReadyService(api, nixl, { pods: 3 }),
    ])
    // The rendered templates, before LWS adds its own variables to the pods.
    const roleEnv = getObject(
      'disaggregatedset',
      namespace,
      tcp.name
    )?.spec.roles.map(
      (role: {
        spec: { leaderWorkerTemplate: { workerTemplate: PodTemplate } }
      }) => role.spec.leaderWorkerTemplate.workerTemplate.spec.containers[0].env
    )
    const tcpEnv = [
      { name: 'MOONCAKE_PROTOCOL', value: 'tcp' },
      { name: 'MC_FORCE_TCP', value: '1' },
      { name: 'E2E_MARK', value: 'pd-tcp' },
    ]
    expect.soft(roleEnv).toEqual([tcpEnv, tcpEnv])
    for (const role of ['prefill', 'decode']) {
      const [tcpPod] = servicePods(namespace, tcp.name, `${roleLabel}=${role}`)
      const tcpRun = engineLaunch(namespace, tcpPod.metadata.name)
      expect.soft(tcpRun.env).toMatchObject({
        MOONCAKE_PROTOCOL: 'tcp',
        MC_FORCE_TCP: '1',
        E2E_MARK: 'pd-tcp',
      })
      expect
        .soft(argValue(tcpRun.argv, '--disaggregation-transfer-backend'))
        .toBe('mooncake')
      expect.soft(tcpRun.argv).not.toContain('--disaggregation-ib-device')

      const [nixlPod] = servicePods(
        namespace,
        nixl.name,
        `${roleLabel}=${role}`
      )
      const nixlRun = engineLaunch(namespace, nixlPod.metadata.name)
      expect
        .soft(argValue(nixlRun.argv, '--disaggregation-transfer-backend'))
        .toBe('nixl')
      expect
        .soft(envOf(runtimeContainer(nixlPod)).MOONCAKE_PROTOCOL)
        .toBeUndefined()
      expect.soft(nixlRun.env.MC_FORCE_TCP).toBeUndefined()
    }
    await waitForRouterWorkers(namespace, tcp.name, { prefill: 1, decode: 1 })
    await deleteServiceAndWaitGone(api, namespace, tcp.name)
    await deleteServiceAndWaitGone(api, namespace, nixl.name)

    const created = await tryCreateService(api, override)
    expect.soft(created.status, created.text).toBe(201)
    if (created.status === 201) {
      await waitForServiceReady(api, namespace, override.name, { pods: 3 })
      for (const role of ['prefill', 'decode']) {
        const [pod] = servicePods(
          namespace,
          override.name,
          `${roleLabel}=${role}`
        )
        expect
          .soft(engineLaunch(namespace, pod.metadata.name).env.MC_FORCE_TCP)
          .toBe('0')
      }
      await deleteServiceAndWaitGone(api, namespace, override.name)
    }
  })

  test('deleting a pd service of an s3 model removes everything created for it', async () => {
    // Deleting must also work in clusters without the PodMonitor CRD.
    expect(crdInstalled(podMonitorCRD)).toBe(false)
    await expectJSON(
      await api.request.put('/api/v1/admin/model-serving/storage', {
        data: {
          endpoint: 'http://object-storage.invalid:9000',
          bucket: 'e2e-models',
          accessKey: 'e2e-access-key',
          secretKey: 'e2e-secret-key',
          // Any local image: the download fails, the objects exist anyway.
          downloaderImage: modelServing.engineImage,
        },
      }),
      200
    )
    const s3ModelId = await api.ensureModel({
      name: 'e2e-s3-chat',
      type: 'chat',
      sourceType: 's3',
      path: 'models/e2e-s3-chat',
    })
    const req = { ...pdService('pd-s3'), modelId: s3ModelId }
    await expectJSON(await api.createService(req), 201)
    const secret = await waitFor<KubeObject>('the model storage Secret', () =>
      getObject('secret', namespace, `${req.name}-model-storage`)
    )
    expect
      .soft(secret.metadata.labels)
      .toMatchObject({ ...managedBy, [serviceLabel]: req.name })
    const prefill = await waitFor(
      'a prefill pod',
      () => servicePods(namespace, req.name, `${roleLabel}=prefill`)[0]
    )
    const [init] =
      (prefill.spec as { initContainers?: Container[] }).initContainers ?? []
    expect.soft(init?.name).toBe('model-downloader')
    expect
      .soft((init as { envFrom?: unknown } | undefined)?.envFrom)
      .toEqual([{ secretRef: { name: `${req.name}-model-storage` } }])
    await waitFor('the role LeaderWorkerSets', () =>
      kubectl([
        'get',
        'leaderworkersets',
        '-n',
        namespace,
        '-l',
        `disaggregatedset.x-k8s.io/name=${req.name}`,
        '-o',
        'name',
      ])
        .trim()
        .split('\n')
        .filter(Boolean).length === 2
        ? true
        : undefined
    )

    // The service is deleted before it ever became ready; the role
    // LeaderWorkerSets go with the DisaggregatedSet through garbage collection.
    await deleteServiceAndWaitGone(api, namespace, req.name)
  })

  test('scaling follows replicas and pd role replicas, and delete cleans up', async () => {
    const single = service({ name: 'scale-single', mode: 'single' })
    const pd = pdService('scale-pd')
    await Promise.all([
      createReadyService(api, single, { pods: 1 }),
      createReadyService(api, pd, { pods: 3 }),
    ])

    const invalid = await api.scaleService(namespace, single.name, {
      replicas: 0,
    })
    expect.soft(invalid.status(), await invalid.text()).toBe(400)
    const scaled = await expectJSON<InferenceService>(
      await api.scaleService(namespace, single.name, { replicas: 2 }),
      200
    )
    expect
      .soft(scaled.roles)
      .toEqual([expect.objectContaining({ name: 'default', replicas: 2 })])
    await waitForServiceReady(api, namespace, single.name, { pods: 2 })
    expect
      .soft(getObject('leaderworkerset', namespace, single.name)?.spec.replicas)
      .toBe(2)
    await expectServiceServesModel(
      single.name,
      servicePods(namespace, single.name)
    )

    const invalidPD = await api.scaleService(namespace, pd.name, {
      prefillReplicas: 0,
      decodeReplicas: 1,
    })
    expect.soft(invalidPD.status(), await invalidPD.text()).toBe(400)
    await expectJSON(
      await api.scaleService(namespace, pd.name, {
        prefillReplicas: 2,
        decodeReplicas: 2,
      }),
      200
    )
    expect
      .soft(
        getObject('disaggregatedset', namespace, pd.name)?.spec.roles.map(
          (role: { name: string; spec: { replicas: number } }) =>
            `${role.name}=${role.spec.replicas}`
        )
      )
      .toEqual(['prefill=2', 'decode=2'])
    await waitForServiceReady(api, namespace, pd.name, { pods: 5 })
    await waitForRouterWorkers(namespace, pd.name, { prefill: 2, decode: 2 })

    // The router follows scale-down too.
    await expectJSON(
      await api.scaleService(namespace, pd.name, {
        prefillReplicas: 1,
        decodeReplicas: 2,
      }),
      200
    )
    await waitForServiceReady(api, namespace, pd.name, { pods: 4 })
    const workers = await waitForRouterWorkers(namespace, pd.name, {
      prefill: 1,
      decode: 2,
    })
    const [prefill] = servicePods(namespace, pd.name, `${roleLabel}=prefill`)
    expect
      .soft(workers.find((item) => item.type === 'prefill')?.url)
      .toBe(podURL(prefill))

    await deleteServiceAndWaitGone(api, namespace, single.name)
    await deleteServiceAndWaitGone(api, namespace, pd.name)
    expect(await api.listServices()).toEqual([])
  })

  test('pd service names longer than 30 characters are rejected', async () => {
    const response = await api.createService(
      pdService('pd-name-that-is-31-characters-x')
    )
    expect(response.status(), await response.text()).toBe(400)
  })

  test('mooncake over rdma is rejected without rdma', async () => {
    const response = await api.createService(
      pdService(
        'pd-no-rdma',
        { transferBackend: 'mooncake' },
        { rdma: undefined }
      )
    )
    expect(response.status(), await response.text()).toBe(400)
  })

  test('the router scales apart from the pd roles', async () => {
    const req = pdService('pd-router', { routerReplicas: 2 })
    const routerName = `${req.name}-router`
    const routerReplicas = () =>
      getObject('deployment', namespace, routerName)?.spec.replicas
    // Every router replica finds the same workers on its own.
    const expectEachRouterFinds = async (expected: {
      prefill: number
      decode: number
    }) => {
      for (const router of servicePods(
        namespace,
        req.name,
        `${roleLabel}=router`
      )) {
        await waitForRouterWorkers(namespace, req.name, expected, {
          pod: router.metadata.name,
        })
      }
    }
    await createReadyService(api, req, { pods: 4 })
    expect.soft(routerReplicas()).toBe(2)
    await expectEachRouterFinds({ prefill: 1, decode: 1 })

    // Leaving routerReplicas out leaves the router alone.
    await expectJSON(
      await api.scaleService(namespace, req.name, {
        prefillReplicas: 1,
        decodeReplicas: 2,
      }),
      200
    )
    await waitForServiceReady(api, namespace, req.name, { pods: 5 })
    expect.soft(routerReplicas()).toBe(2)
    await expectEachRouterFinds({ prefill: 1, decode: 2 })

    const scaled = await expectJSON<InferenceService>(
      await api.scaleService(namespace, req.name, {
        prefillReplicas: 1,
        decodeReplicas: 2,
        routerReplicas: 1,
      }),
      200
    )
    expect
      .soft(scaled.roles)
      .toEqual(
        expect.arrayContaining([
          expect.objectContaining({ name: 'router', replicas: 1 }),
        ])
      )
    expect.soft(routerReplicas()).toBe(1)
    await waitForServiceReady(api, namespace, req.name, { pods: 4 })
    await expectEachRouterFinds({ prefill: 1, decode: 2 })

    const invalid = await api.scaleService(namespace, req.name, {
      prefillReplicas: 1,
      decodeReplicas: 2,
      routerReplicas: 0,
    })
    expect.soft(invalid.status(), await invalid.text()).toBe(400)

    // Without its router, a service cannot scale one, and nothing changes.
    kubectl(['delete', 'deployment', routerName, '-n', namespace])
    const missing = await api.scaleService(namespace, req.name, {
      prefillReplicas: 2,
      decodeReplicas: 2,
      routerReplicas: 2,
    })
    expect.soft(missing.status(), await missing.text()).toBe(400)
    expect
      .soft(
        getObject('disaggregatedset', namespace, req.name)?.spec.roles.map(
          (role: { name: string; spec: { replicas: number } }) =>
            `${role.name}=${role.spec.replicas}`
        )
      )
      .toEqual(['prefill=1', 'decode=2'])

    await deleteServiceAndWaitGone(api, namespace, req.name)
  })

  test('a cluster with the PodMonitor CRD gets a PodMonitor per service that scrapes leaders', async () => {
    installPrometheusCRDs()
    // PodMonitors take the labels the cluster's Prometheus selects them by.
    kubectl(['apply', '-n', namespace, '-f', '-'], {
      input: JSON.stringify({
        apiVersion: 'monitoring.coreos.com/v1',
        kind: 'Prometheus',
        metadata: { name: 'e2e-prometheus' },
        spec: { podMonitorSelector: { matchLabels: { prometheus: 'e2e' } } },
      }),
    })
    const req = pdService('pd-metrics', {
      prefill: { replicas: 1, groupSize: 2, gpuCount: 1 },
      decode: { replicas: 1, groupSize: 1, gpuCount: 1 },
    })
    await createReadyService(api, req, { pods: 4 })
    const monitor = getObject('podmonitor', namespace, req.name)
    expect(monitor).toBeTruthy()
    expect.soft(monitor?.metadata.labels).toEqual({
      ...managedBy,
      [serviceLabel]: req.name,
      release: 'kube-prometheus-stack',
      prometheus: 'e2e',
    })
    const selector = monitor?.spec.selector.matchLabels as Record<
      string,
      string
    >
    expect.soft(selector).toEqual({
      ...managedBy,
      [serviceLabel]: req.name,
      [workerIndexLabel]: '0',
    })
    expect
      .soft(monitor?.spec.podMetricsEndpoints)
      .toEqual([expect.objectContaining({ port: 'http', path: '/metrics' })])

    // It selects the prefill and decode leaders: not the prefill worker,
    // which serves no HTTP, nor the router.
    const selected = kubectlJSON<{ items: Pod[] }>([
      'get',
      'pods',
      '-n',
      namespace,
      '-l',
      Object.entries(selector)
        .map(([key, value]) => `${key}=${value}`)
        .join(','),
    ]).items
    const leaders = [
      leaderOf(servicePods(namespace, req.name, `${roleLabel}=prefill`)),
      ...servicePods(namespace, req.name, `${roleLabel}=decode`),
    ]
    expect
      .soft(selected.map((pod) => pod.metadata.name).sort())
      .toEqual(leaders.map((pod) => pod.metadata.name).sort())
    // What they serve there is what the inference dashboards query.
    for (const pod of selected) {
      const metrics = kubectl([
        'get',
        '--raw',
        `/api/v1/namespaces/${namespace}/pods/${pod.metadata.name}:8000/proxy/metrics`,
      ])
      expect.soft(metrics).toContain('sglang:num_running_reqs')
      expect
        .soft(metrics)
        .toContain('sglang:time_to_first_token_seconds_bucket')
    }

    // The PodMonitor goes with the service; deleteServiceAndWaitGone checks
    // PodMonitors whenever their CRD is installed.
    await deleteServiceAndWaitGone(api, namespace, req.name)
    expect(getObject('podmonitor', namespace, req.name)).toBeUndefined()
  })

  test('vllm pd points decode at the NIXL side channel of prefill', async () => {
    const req = pdService(
      'vllm-pd',
      { routerImage: modelServing.vllmRouterImage, transferBackend: 'nixl' },
      { runtime: 'vllm', rdma: undefined }
    )
    await createReadyService(api, req, { pods: 3 })
    const [prefill] = servicePods(namespace, req.name, `${roleLabel}=prefill`)
    const [decode] = servicePods(namespace, req.name, `${roleLabel}=decode`)
    for (const [pod, kvRole] of [
      [prefill, 'kv_producer'],
      [decode, 'kv_consumer'],
    ] as const) {
      const run = engineLaunch(namespace, pod.metadata.name)
      expect
        .soft(JSON.parse(argValue(run.argv, '--kv-transfer-config') ?? '{}'))
        .toEqual({ kv_connector: 'NixlConnector', kv_role: kvRole })
      expect.soft(run.env.VLLM_NIXL_SIDE_CHANNEL_HOST).toBe(pod.status.podIP)
      expect
        .soft(runtimeContainer(pod).ports?.map((port) => port.containerPort))
        .toEqual([8000, 5600])
    }
    const [router] = servicePods(namespace, req.name, `${roleLabel}=router`)
    expect.soft(runtimeContainer(router).command).toEqual(['vllm-router'])
    expect.soft(runtimeContainer(router).env).toBeUndefined()

    const workers = await waitForRouterWorkers(namespace, req.name, {
      prefill: 1,
      decode: 1,
    })
    expect
      .soft(workers.map((item) => item.url).sort())
      .toEqual([podURL(prefill), podURL(decode)].sort())

    const reply = JSON.parse(
      postFromPod(
        namespace,
        prefill.metadata.name,
        `http://${req.name}.${namespace}.svc:8000/v1/chat/completions`,
        {
          model: modelName,
          messages: [{ role: 'user', content: 'hello' }],
          max_tokens: 4,
        }
      )
    )
    expect.soft(reply.choices?.[0]?.message?.content).toBe('fake engine reply')
    const prefillChat = engineRequests(namespace, prefill.metadata.name).find(
      (item) => item.path === '/v1/chat/completions'
    )
    expect
      .soft(prefillChat?.body?.kv_transfer_params)
      .toMatchObject({ do_remote_decode: true })
    expect.soft(engineKVTransfers(namespace, decode.metadata.name)).toEqual([
      expect.objectContaining({
        host: prefill.status.podIP,
        port: 5600,
        connected: true,
      }),
    ])
    const routerLog = kubectl(['logs', '-n', namespace, router.metadata.name])
    expect.soft(routerLog).not.toMatch(/forbidden/i)
    expect.soft(routerLog).not.toMatch(/huggingface|tokenizer/i)

    await deleteServiceAndWaitGone(api, namespace, req.name)
  })
})
