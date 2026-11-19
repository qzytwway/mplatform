import { createContext, useContext } from 'react'

interface GlobalSearchContextType {
  isOpen: boolean
  openSearch: () => void
  closeSearch: () => void
  toggleSearch: () => void
}

export const GlobalSearchContext = createContext<
  GlobalSearchContextType | undefined
>(undefined)

export function useGlobalSearch() {
  const context = useContext(GlobalSearchContext)
  if (context === undefined) {
    throw new Error(
      'useGlobalSearch must be used within a GlobalSearchProvider'
    )
  }
  return context
}
