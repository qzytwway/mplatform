import { RefreshCcw } from 'lucide-react'

export const ConnectionIndicator: React.FC<{
  isConnected: boolean
  onReconnect?: () => void
  children?: React.ReactNode
}> = ({ isConnected, onReconnect, children }) => {
  if (isConnected) {
    return (
      <div className="flex items-center gap-1.5">
        <div className="w-2.5 h-2.5 rounded-full bg-success breathing-indicator" />
        {children}
      </div>
    )
  } else {
    return (
      <div className="flex items-center gap-1.5">
        <div className="w-2.5 h-2.5 rounded-full bg-destructive breathing-indicator" />
        {children}
        {onReconnect && (
          <button
            onClick={onReconnect}
            className="p-1 hover:bg-muted rounded-full text-muted-foreground hover:text-foreground"
          >
            <RefreshCcw className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    )
  }
}
