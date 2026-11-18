import { useState } from 'react'

/**
 * Returns true on the first render and on every render where a value in
 * `deps` differs (Object.is) from the previous render. Use it to adjust state
 * during render when props change, instead of calling setState in an effect:
 * https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
 *
 * Deps must keep their identity across renders: an array or object rebuilt on
 * every render (e.g. a `= []` default) re-renders forever. Pass query data or
 * a string key instead.
 */
export function useDepsChanged(deps: readonly unknown[]): boolean {
  const [prevDeps, setPrevDeps] = useState<readonly unknown[] | null>(null)
  const changed =
    prevDeps === null ||
    prevDeps.length !== deps.length ||
    deps.some((dep, index) => !Object.is(dep, prevDeps[index]))
  if (changed) setPrevDeps(deps)
  return changed
}
