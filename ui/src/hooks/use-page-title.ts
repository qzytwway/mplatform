import { useEffect } from 'react'

export function usePageTitle(title: string) {
  useEffect(() => {
    const previousTitle = document.title

    if (title) {
      document.title = `${title} - MPlatform`
    }

    return () => {
      document.title = previousTitle
    }
  }, [title])
}
