import { describe, expect, it } from 'vitest'

import { layoutPanels } from './layout'
import type { GrafanaPanel } from './types'

const panel = (x: number, y: number, w: number, h: number): GrafanaPanel => ({
  type: 'timeseries',
  gridPos: { x, y, w, h },
})

describe('layoutPanels', () => {
  it('pushes panels with overlapping saved positions below each other', () => {
    const panels = [
      panel(0, 0, 4, 3),
      panel(4, 0, 4, 3),
      panel(0, 1, 24, 7),
      panel(0, 2, 24, 7),
      panel(0, 3, 24, 7),
    ]

    const ys = layoutPanels(panels, {}).map((p) => p.gridPos.y)

    expect(ys).toEqual([0, 0, 3, 10, 17])
  })

  it('still closes gaps above a panel', () => {
    const ys = layoutPanels([panel(0, 0, 24, 3), panel(0, 10, 24, 3)], {}).map(
      (p) => p.gridPos.y
    )

    expect(ys).toEqual([0, 3])
  })
})
