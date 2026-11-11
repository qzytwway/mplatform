import { describe, expect, it } from 'vitest'

import type { ModelServingStatus } from '@/lib/api/model-serving'

import { modeBlocker } from './service-modes'

const status = (disaggregatedSet: boolean): ModelServingStatus => ({
  lws: true,
  disaggregatedSet,
  installed: true,
  chart: { name: 'lws' },
})

describe('modeBlocker', () => {
  it.each([
    ['single for embeddings', 'single', 'embedding', false, undefined],
    ['multi for chat', 'multi', 'chat', false, undefined],
    [
      'multi for embeddings',
      'multi',
      'embedding',
      true,
      'modelServing.services.modeBlockers.chatOnly',
    ],
    // Either runtime serves PD once DisaggregatedSet is installed.
    ['pd for chat', 'pd', 'chat', true, undefined],
    ['pd before a model is chosen', 'pd', undefined, true, undefined],
    [
      'pd without DisaggregatedSet',
      'pd',
      'chat',
      false,
      'modelServing.services.modeBlockers.pdNotInstalled',
    ],
    [
      'pd for embeddings',
      'pd',
      'embedding',
      true,
      'modelServing.services.modeBlockers.chatOnly',
    ],
  ] as const)('%s', (_, mode, modelType, installed, want) => {
    expect(modeBlocker(mode, modelType, status(installed))).toBe(want)
  })

  it('blocks pd while the status loads', () => {
    expect(modeBlocker('pd', 'chat', undefined)).toBe(
      'modelServing.services.modeBlockers.pdNotInstalled'
    )
  })
})
