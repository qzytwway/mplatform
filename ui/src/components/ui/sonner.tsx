import { useTheme } from 'next-themes'
import { Toaster as Sonner, ToasterProps } from 'sonner'

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = 'system' } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps['theme']}
      className="toaster group"
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          error: '[&_svg]:!text-destructive',
          success: '[&_svg]:!text-success',
          warning: '[&_svg]:!text-warning',
          info: '[&_svg]:!text-info',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
