import { useMemo } from 'react'

import { useDisplayScale } from './use-display-scale'
import { useTheme } from './use-theme'

// Unified hook for reading/updating all appearance settings in one place.
export function useAppearance() {
  const { theme, actualTheme, setTheme } = useTheme()
  const { displayScale, setDisplayScale } = useDisplayScale()

  return useMemo(
    () => ({
      theme,
      actualTheme,
      setTheme,
      displayScale,
      setDisplayScale,
    }),
    [theme, actualTheme, displayScale, setTheme, setDisplayScale]
  )
}
