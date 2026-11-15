import type { Engine } from '@/lib/api/model-serving'

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
}

export const ENGINES = [VLLM_ENGINE, SGLANG_ENGINE]
