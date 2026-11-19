import { createContext, useContext } from 'react'

type DisplayScaleProviderState = {
  displayScale: number
  setDisplayScale: (displayScale: number) => void
}

const initialState: DisplayScaleProviderState = {
  displayScale: 95,
  setDisplayScale: () => null,
}

export const DisplayScaleProviderContext =
  createContext<DisplayScaleProviderState>(initialState)

export const useDisplayScale = () => {
  const context = useContext(DisplayScaleProviderContext)
  if (context === undefined)
    throw new Error(
      'useDisplayScale must be used within a DisplayScaleProvider'
    )
  return context
}
