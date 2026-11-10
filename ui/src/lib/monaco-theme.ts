import { useEffect, useState } from 'react'
import { formatHex } from 'culori'

type MonacoModule = typeof import('monaco-editor')

const definedThemeSignatures = new Map<string, string>()

function getDefaultBackground(actualTheme: 'dark' | 'light') {
  return actualTheme === 'dark' ? '#191817' : '#ffffff'
}

function defineThemeIfNeeded(
  monaco: MonacoModule,
  themeName: string,
  signature: string,
  definition: Parameters<MonacoModule['editor']['defineTheme']>[1]
) {
  if (definedThemeSignatures.get(themeName) === signature) {
    return
  }

  monaco.editor.defineTheme(themeName, definition)
  definedThemeSignatures.set(themeName, signature)
}

export function getMonacoBackgroundColor(
  cssVariableName: '--background' | '--card',
  actualTheme: 'dark' | 'light'
) {
  const background = getComputedStyle(document.documentElement)
    .getPropertyValue(cssVariableName)
    .trim()

  return formatHex(background) || getDefaultBackground(actualTheme)
}

export function useMonacoBackgroundColor(
  cssVariableName: '--background' | '--card',
  actualTheme: 'dark' | 'light'
) {
  const [backgroundColor, setBackgroundColor] = useState(() =>
    getMonacoBackgroundColor(cssVariableName, actualTheme)
  )

  useEffect(() => {
    const updateBackground = () => {
      setBackgroundColor(getMonacoBackgroundColor(cssVariableName, actualTheme))
    }
    const frame = requestAnimationFrame(updateBackground)

    return () => cancelAnimationFrame(frame)
  }, [cssVariableName, actualTheme])

  return backgroundColor
}

export function defineMonacoBackgroundThemes(
  monaco: MonacoModule,
  {
    darkThemeName,
    lightThemeName,
    backgroundColor,
  }: {
    darkThemeName: string
    lightThemeName: string
    backgroundColor: string
  }
) {
  defineThemeIfNeeded(
    monaco,
    darkThemeName,
    `${darkThemeName}:${backgroundColor}`,
    {
      base: 'vs-dark',
      inherit: true,
      rules: [],
      colors: {
        'editor.background': backgroundColor,
      },
    }
  )

  defineThemeIfNeeded(
    monaco,
    lightThemeName,
    `${lightThemeName}:${backgroundColor}`,
    {
      base: 'vs',
      inherit: true,
      rules: [],
      colors: {
        'editor.background': backgroundColor,
      },
    }
  )
}
