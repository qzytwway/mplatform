import type { Engine, ServiceOptions } from '@/lib/api/model-serving'

// The Engine catalog the backend serves, for tests of forms that follow it.
export const VLLM_ENGINE: Engine = {
  name: 'vllm',
  label: 'vLLM',
  defaultImage: 'vllm/vllm-openai:latest',
  pdImage: 'vllm/vllm-openai:v0.33.0',
  defaultRouterImage: 'vllm/vllm-router:v0.1.17',
  transferBackends: ['nixl'],
  defaultTransfer: 'nixl',
  defaultRDMATransfer: 'nixl',
  pdSingleNode: true,
  pdSameImage: true,
  pipelineAcrossNodes: true,
  roleHints: {
    prefill: {
      extraArgs: '--max-num-batched-tokens 16384',
      env: 'UCX_NET_DEVICES=mlx5_0:1',
    },
    decode: {
      extraArgs: '--max-num-seqs 128',
      env: 'UCX_NET_DEVICES=mlx5_0:1',
    },
  },
  toolCallParsers: [{ value: 'hermes', models: 'Qwen2.5 / Qwen3' }],
  reasoningParsers: [{ value: 'qwen3', models: 'Qwen3' }],
}

export const SGLANG_ENGINE: Engine = {
  name: 'sglang',
  label: 'SGLang',
  defaultImage: 'lmsysorg/sglang:latest',
  pdImage: 'lmsysorg/sglang:latest',
  defaultRouterImage: '',
  transferBackends: ['mooncake', 'mooncake-tcp', 'nixl'],
  defaultTransfer: 'mooncake-tcp',
  defaultRDMATransfer: 'mooncake',
  pdSingleNode: false,
  pdSameImage: false,
  pipelineAcrossNodes: false,
  roleHints: {
    prefill: {
      extraArgs: '--chunked-prefill-size 16384',
      env: 'SGLANG_DISAGGREGATION_THREAD_POOL_SIZE=4',
    },
    decode: {
      extraArgs: '--max-running-requests 128',
      env: 'SGLANG_DISAGGREGATION_QUEUE_SIZE=4',
    },
  },
  toolCallParsers: [{ value: 'qwen25', models: 'Qwen2.5 / Qwen3' }],
  reasoningParsers: [{ value: 'auto', models: '' }],
}

export const ENGINES = [VLLM_ENGINE, SGLANG_ENGINE]

export const SERVICE_OPTIONS: ServiceOptions = {
  engines: ENGINES,
  routerPolicies: ['cache_aware', 'power_of_two', 'round_robin', 'random'],
  maxNameLength: 40,
  maxPDNameLength: 30,
  maxPDRoleReplicas: 100,
}
