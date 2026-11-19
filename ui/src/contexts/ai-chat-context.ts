import { createContext, useContext } from 'react'

import { PageContext } from '@/components/ai-chat/ai-chat-types'

export interface AIChatContextType {
  isOpen: boolean
  openChat: () => void
  closeChat: () => void
  toggleChat: () => void
  pageContext: PageContext
}

export const AIChatContext = createContext<AIChatContextType | undefined>(
  undefined
)

export function useAIChatContext() {
  const context = useContext(AIChatContext)
  if (context === undefined) {
    throw new Error('useAIChatContext must be used within an AIChatProvider')
  }
  return context
}
