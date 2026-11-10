/* eslint-disable react-refresh/only-export-components */
import { useMemo, type ReactNode } from 'react'

import { DisplayScaleProvider, useDisplayScale } from './display-scale-provider'
import { ThemeProvider, useTheme } from './theme-provider'

type AppearanceProviderProps = {
  children: ReactNode
  // Theme
  defaultTheme?: 'dark' | 'light' | 'system'
  themeStorageKey?: string
  // Display scale
  defaultDisplayScale?: number
  displayScaleStorageKey?: string
}

export function AppearanceProvider({
  children,
  defaultTheme = 'system',
  themeStorageKey = 'vite-ui-theme',
  defaultDisplayScale = 95,
  displayScaleStorageKey = 'vite-ui-display-scale',
}: AppearanceProviderProps) {
  return (
    <ThemeProvider defaultTheme={defaultTheme} storageKey={themeStorageKey}>
      <DisplayScaleProvider
        defaultDisplayScale={defaultDisplayScale}
        storageKey={displayScaleStorageKey}
      >
        {children}
      </DisplayScaleProvider>
    </ThemeProvider>
  )
}

export default AppearanceProvider

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
