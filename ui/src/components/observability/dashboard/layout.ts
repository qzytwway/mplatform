import type { GrafanaPanel, GridPos } from './types'
import { ALL_VALUE, type VariableState } from './variables'

export interface PanelInstance {
  key: string
  panel: GrafanaPanel
  gridPos: GridPos
  repeat?: { name: string; value: string }
}

const DEFAULT_GRID_POS: GridPos = { x: 0, y: 0, w: 12, h: 8 }

// Places panels on Grafana's 24-column grid. A repeated panel gets a copy per
// selected value of its variable, stacked under the first, and the panels
// below move down to make room.
export function layoutPanels(
  panels: GrafanaPanel[],
  variables: Record<string, VariableState>
): PanelInstance[] {
  const placed: (PanelInstance & { originalY: number })[] = []
  const shifts = new Map<number, number>()
  panels.forEach((panel, index) => {
    const gridPos = panel.gridPos ?? DEFAULT_GRID_POS
    if (!panel.repeat) {
      placed.push({ key: String(index), panel, gridPos, originalY: gridPos.y })
      return
    }
    const state = variables[panel.repeat]
    const values = !state
      ? []
      : state.value === ALL_VALUE
        ? state.options
        : [state.value]
    values.forEach((value, copy) => {
      placed.push({
        key: `${index}-${value}`,
        panel,
        gridPos: { ...gridPos, y: gridPos.y + copy * gridPos.h },
        repeat: { name: panel.repeat!, value },
        originalY: gridPos.y,
      })
    })
    const below = gridPos.y + gridPos.h
    const added = Math.max(0, values.length - 1) * gridPos.h
    shifts.set(below, Math.max(shifts.get(below) ?? 0, added))
  })
  return placed
    .map(({ originalY, ...instance }) => {
      let shift = 0
      for (const [below, added] of shifts) {
        if (originalY >= below) shift += added
      }
      return {
        ...instance,
        gridPos: { ...instance.gridPos, y: instance.gridPos.y + shift },
      }
    })
    .sort((a, b) => a.gridPos.y - b.gridPos.y || a.gridPos.x - b.gridPos.x)
    .reduce<PanelInstance[]>(compact, [])
}

const overlaps = (a: GridPos, b: GridPos) =>
  a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h

// Moves a panel up into free space above it, as Grafana's grid does, so
// gaps in the saved positions close.
function compact(placed: PanelInstance[], instance: PanelInstance) {
  let gridPos = instance.gridPos
  while (gridPos.y > 0) {
    const above = { ...gridPos, y: gridPos.y - 1 }
    if (placed.some((other) => overlaps(above, other.gridPos))) break
    gridPos = above
  }
  return [...placed, { ...instance, gridPos }]
}
