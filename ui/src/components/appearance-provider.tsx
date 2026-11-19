import { type ReactNode } from 'react'

import { DisplayScaleProvider } from './display-scale-provider'
import { ThemeProvider } from './theme-provider'

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
