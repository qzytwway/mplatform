import { ReactNode, useCallback, useEffect, useState } from 'react'

import { GlobalSearchContext } from '@/hooks/use-global-search'

interface GlobalSearchProviderProps {
  children: ReactNode
}

export function GlobalSearchProvider({ children }: GlobalSearchProviderProps) {
  const [isOpen, setIsOpen] = useState(false)

  const openSearch = () => setIsOpen(true)
  const closeSearch = () => setIsOpen(false)
  const toggleSearch = useCallback(() => setIsOpen((prev) => !prev), [])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Command+K or Ctrl+K to open search
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        toggleSearch()
      }

      // Escape to close search
      if (e.key === 'Escape' && isOpen) {
        closeSearch()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, toggleSearch])

  const value = {
    isOpen,
    openSearch,
    closeSearch,
    toggleSearch,
  }

  return (
    <GlobalSearchContext.Provider value={value}>
      {children}
    </GlobalSearchContext.Provider>
  )
}
