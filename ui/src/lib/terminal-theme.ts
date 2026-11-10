import type { ITheme } from '@xterm/xterm'
import { formatHex, formatHex8, parse } from 'culori'

// xterm cannot parse oklch, so resolve the theme tokens to hex at the moment of use.
export function getTerminalTheme(): ITheme {
  const style = getComputedStyle(document.documentElement)
  const color = (name: string, alpha?: number) => {
    const parsed = parse(style.getPropertyValue(`--${name}`).trim())
    if (!parsed) return undefined
    return alpha === undefined
      ? formatHex(parsed)
      : formatHex8({ ...parsed, alpha })
  }

  return {
    background: color('card'),
    foreground: color('foreground'),
    cursor: color('foreground'),
    selectionBackground: color('foreground', 0.2),
    black: color('ansi-black'),
    red: color('ansi-red'),
    green: color('ansi-green'),
    yellow: color('ansi-yellow'),
    blue: color('ansi-blue'),
    magenta: color('ansi-magenta'),
    cyan: color('ansi-cyan'),
    white: color('ansi-white'),
    brightBlack: color('ansi-bright-black'),
    brightRed: color('ansi-bright-red'),
    brightGreen: color('ansi-bright-green'),
    brightYellow: color('ansi-bright-yellow'),
    brightBlue: color('ansi-bright-blue'),
    brightMagenta: color('ansi-bright-magenta'),
    brightCyan: color('ansi-bright-cyan'),
    brightWhite: color('ansi-bright-white'),
  }
}
