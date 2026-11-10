/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useEffect, useMemo, useState } from 'react'

type FontOption = 'system'

type FontProviderProps = {
  children: React.ReactNode
  defaultFont?: FontOption
  storageKey?: string
}

type FontProviderState = {
  font: FontOption
  setFont: (font: FontOption) => void
}

const initialState: FontProviderState = {
  font: 'system',
  setFont: () => null,
}

const FontProviderContext = createContext<FontProviderState>(initialState)

export function FontProvider({
  children,
  defaultFont = 'system',
  storageKey = 'vite-ui-font',
  ...props
}: FontProviderProps) {
  const [font, setFont] = useState<FontOption>(defaultFont)

  useEffect(() => {
    const root = window.document.documentElement
    root.style.setProperty('--app-font-sans', 'var(--font-sans)')
    localStorage.setItem(storageKey, font)
  }, [font, storageKey])

  const value = useMemo(
    () => ({
      font,
      setFont: (f: FontOption) => setFont(f),
    }),
    [font]
  )

  return (
    <FontProviderContext.Provider {...props} value={value}>
      {children}
    </FontProviderContext.Provider>
  )
}

export const useFont = () => {
  const context = useContext(FontProviderContext)
  if (context === undefined)
    throw new Error('useFont must be used within a FontProvider')
  return context
}
