import type { YamlEditorProps } from '@mplatform/plugin-sdk/ui'

import { MonacoEditor } from '@/lib/monaco-loader'
import {
  defineMonacoBackgroundThemes,
  useMonacoBackgroundColor,
} from '@/lib/monaco-theme'

import { useAppearance } from './appearance-provider'

export function SimpleYamlEditor({
  value,
  onChange,
  disabled = false,
  height = '400px',
}: YamlEditorProps) {
  const { actualTheme } = useAppearance()
  const themeMode = actualTheme === 'dark' ? 'dark' : 'light'
  const backgroundColor = useMonacoBackgroundColor('--background', themeMode)
  return (
    <div className="border rounded-md overflow-hidden">
      <MonacoEditor
        key={`simple-yaml-editor-${actualTheme}-${backgroundColor}`}
        height={height}
        defaultLanguage="yaml"
        value={value}
        onChange={onChange}
        loading={
          <div
            className="flex items-center justify-center h-full text-muted-foreground"
            style={{ height }}
          >
            Loading editor...
          </div>
        }
        beforeMount={(monaco) => {
          defineMonacoBackgroundThemes(monaco, {
            darkThemeName: 'custom-dark',
            lightThemeName: 'custom-vs',
            backgroundColor,
          })
        }}
        theme={actualTheme === 'dark' ? 'custom-dark' : 'custom-vs'}
        options={{
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          wordWrap: 'on',
          readOnly: disabled,
          fontSize: 14,
          lineNumbers: 'on',
          folding: true,
          autoIndent: 'full',
          formatOnPaste: true,
          formatOnType: true,
          tabSize: 2,
          insertSpaces: true,
          detectIndentation: true,
          renderWhitespace: 'boundary',
          scrollbar: {
            verticalScrollbarSize: 8,
            horizontalScrollbarSize: 8,
          },
          fontFamily:
            "Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace",
        }}
      />
    </div>
  )
}
