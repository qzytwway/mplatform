import { createContext, useContext } from 'react'

export interface TerminalContextType {
  isOpen: boolean
  isMinimized: boolean
  openTerminal: () => void
  closeTerminal: () => void
  minimizeTerminal: () => void
  toggleTerminal: () => void
}

export const TerminalContext = createContext<TerminalContextType | undefined>(
  undefined
)

export function useTerminal() {
  const context = useContext(TerminalContext)
  if (context === undefined) {
    throw new Error('useTerminal must be used within a TerminalProvider')
  }
  return context
}
