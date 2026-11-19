import { ReactNode, useCallback, useMemo, useState } from 'react'
import { useLocation, useParams } from 'react-router-dom'

import { PageContext } from '@/components/ai-chat/ai-chat-types'
import { toSingularResource } from '@/components/ai-chat/ai-chat-utils'

import { AIChatContext } from './ai-chat-context'

export function AIChatProvider({ children }: { children: ReactNode }) {
  const [isOpen, setIsOpen] = useState(false)
  const location = useLocation()
  const params = useParams()

  const openChat = useCallback(() => {
    setIsOpen(true)
  }, [])

  const closeChat = useCallback(() => {
    setIsOpen(false)
  }, [])

  const toggleChat = useCallback(() => {
    setIsOpen((prev) => !prev)
  }, [])

  const pageContext = useMemo<PageContext>(() => {
    const path = location.pathname
    const searchParams = new URLSearchParams(location.search)

    if (path === '/ai-chat-box') {
      return {
        page: searchParams.get('page') || 'overview',
        namespace: searchParams.get('namespace') || '',
        resourceName: searchParams.get('resourceName') || '',
        resourceKind: toSingularResource(
          searchParams.get('resourceKind') || ''
        ),
      }
    }

    const resource = params.resource || ''
    const name = params.name || ''
    const namespace = params.namespace || ''
    const normalizedKind = toSingularResource(resource)

    let page = 'overview'
    if (path === '/' || path === '/dashboard') {
      page = 'overview'
    } else if (name) {
      page = `${normalizedKind}-detail`
    } else if (resource) {
      page = `${resource}-list`
    }

    return {
      page,
      namespace,
      resourceName: name,
      resourceKind: normalizedKind,
    }
  }, [
    location.pathname,
    location.search,
    params.resource,
    params.name,
    params.namespace,
  ])

  return (
    <AIChatContext.Provider
      value={{
        isOpen,
        openChat,
        closeChat,
        toggleChat,
        pageContext,
      }}
    >
      {children}
    </AIChatContext.Provider>
  )
}
