import { describe, expect, it } from 'vitest'

import { parseAnsi } from './ansi-parser'

const ESC = String.fromCharCode(0x1b)

describe('parseAnsi', () => {
  it('returns plain text as one unstyled segment', () => {
    expect(parseAnsi('hello')).toEqual({
      segments: [{ text: 'hello', styles: {} }],
      finalState: {},
    })
  })

  it('applies SGR colors and resets', () => {
    const { segments, finalState } = parseAnsi(
      `a${ESC}[31mred${ESC}[1;42mboth${ESC}[0mplain`
    )
    expect(segments).toEqual([
      { text: 'a', styles: {} },
      { text: 'red', styles: { color: 'var(--ansi-red)' } },
      {
        text: 'both',
        styles: {
          color: 'var(--ansi-red)',
          bold: true,
          backgroundColor: 'var(--ansi-green)',
        },
      },
      { text: 'plain', styles: {} },
    ])
    expect(finalState).toEqual({})
  })

  it('treats an empty SGR as reset and carries state across calls', () => {
    const first = parseAnsi(`${ESC}[94mx`)
    expect(first.finalState).toEqual({ color: 'var(--ansi-bright-blue)' })
    const second = parseAnsi(`y${ESC}[mz`, first.finalState)
    expect(second.segments).toEqual([
      { text: 'y', styles: { color: 'var(--ansi-bright-blue)' } },
      { text: 'z', styles: {} },
    ])
  })

  it('treats empty codes inside a list as reset', () => {
    expect(parseAnsi(`${ESC}[1;;4mx`).finalState).toEqual({ underline: true })
  })

  it('leaves non-SGR and unterminated escapes in the text', () => {
    expect(parseAnsi(`a${ESC}[2Kb${ESC}[3`).segments).toEqual([
      { text: `a${ESC}[2Kb${ESC}[3`, styles: {} },
    ])
  })

  it('drops escapes that end the text without adding a segment', () => {
    expect(parseAnsi(`x${ESC}[3m`)).toEqual({
      segments: [{ text: 'x', styles: {} }],
      finalState: { italic: true },
    })
  })

  it('applies carriage returns and backspaces', () => {
    expect(parseAnsi('abc\rX\nde\bF').segments).toEqual([
      { text: 'Xbc\ndF', styles: {} },
    ])
  })
})
