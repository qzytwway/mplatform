/* eslint-disable react-refresh/only-export-components */
import { useMemo, type ReactNode } from 'react'

import { DisplayScaleProvider, useDisplayScale } from './display-scale-provider'
import { FontProvider, useFont } from './font-provider'
import { ThemeProvider, useTheme } from './theme-provider'

type AppearanceProviderProps = {
  children: ReactNode
  // Theme
  defaultTheme?: 'dark' | 'light' | 'system'
  themeStorageKey?: string
  // Font
  defaultFont?: 'system'
  fontStorageKey?: string
  // Display scale
  defaultDisplayScale?: number
  displayScaleStorageKey?: string
}

export function AppearanceProvider({
  children,
  defaultTheme = 'system',
  themeStorageKey = 'vite-ui-theme',
  defaultFont = 'system',
  fontStorageKey = 'vite-ui-font',
  defaultDisplayScale = 95,
  displayScaleStorageKey = 'vite-ui-display-scale',
}: AppearanceProviderProps) {
  return (
    <ThemeProvider defaultTheme={defaultTheme} storageKey={themeStorageKey}>
      <DisplayScaleProvider
        defaultDisplayScale={defaultDisplayScale}
        storageKey={displayScaleStorageKey}
      >
        <FontProvider defaultFont={defaultFont} storageKey={fontStorageKey}>
          {children}
        </FontProvider>
      </DisplayScaleProvider>
    </ThemeProvider>
  )
}

export default AppearanceProvider

// Unified hook for reading/updating all appearance settings in one place.
export function useAppearance() {
  const { theme, actualTheme, setTheme } = useTheme()
  const { displayScale, setDisplayScale } = useDisplayScale()
  const { font, setFont } = useFont()

  return useMemo(
    () => ({
      theme,
      actualTheme,
      setTheme,
      displayScale,
      setDisplayScale,
      font,
      setFont,
    }),
    [theme, actualTheme, displayScale, font, setTheme, setDisplayScale, setFont]
  )
}
