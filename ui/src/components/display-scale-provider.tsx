import { useEffect, useMemo, useState } from 'react'

import { DisplayScaleProviderContext } from '@/hooks/use-display-scale'

const MIN_DISPLAY_SCALE = 80
const MAX_DISPLAY_SCALE = 120
const DISPLAY_SCALE_STEP = 5

type DisplayScaleProviderProps = {
  children: React.ReactNode
  defaultDisplayScale?: number
  storageKey?: string
}

function clampDisplayScale(value: number) {
  return Math.min(MAX_DISPLAY_SCALE, Math.max(MIN_DISPLAY_SCALE, value))
}

function normalizeDisplayScale(value: number) {
  return clampDisplayScale(
    Math.round(value / DISPLAY_SCALE_STEP) * DISPLAY_SCALE_STEP
  )
}

export function DisplayScaleProvider({
  children,
  defaultDisplayScale = 95,
  storageKey = 'vite-ui-display-scale',
  ...props
}: DisplayScaleProviderProps) {
  const [displayScale, setDisplayScaleState] = useState(() => {
    const stored = localStorage.getItem(storageKey)
    const value = stored === null ? defaultDisplayScale : Number(stored)

    return Number.isFinite(value)
      ? normalizeDisplayScale(value)
      : normalizeDisplayScale(defaultDisplayScale)
  })

  useEffect(() => {
    window.document.documentElement.style.setProperty(
      '--app-display-scale',
      `${displayScale}%`
    )
    localStorage.setItem(storageKey, String(displayScale))
  }, [displayScale, storageKey])

  const value = useMemo(
    () => ({
      displayScale,
      setDisplayScale: (nextDisplayScale: number) => {
        setDisplayScaleState(normalizeDisplayScale(nextDisplayScale))
      },
    }),
    [displayScale]
  )

  return (
    <DisplayScaleProviderContext.Provider {...props} value={value}>
      {children}
    </DisplayScaleProviderContext.Provider>
  )
}
