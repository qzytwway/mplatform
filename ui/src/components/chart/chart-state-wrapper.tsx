import React from 'react'
import { AlertTriangle, Loader2 } from 'lucide-react'

import { Alert, AlertDescription } from '../ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card'
import { Skeleton } from '../ui/skeleton'

interface ChartStateWrapperProps {
  title: string
  isLoading?: boolean
  error?: Error | null
  isEmpty?: boolean
  cardClassName?: string
  contentClassName?: string
  emptyMessage?: string
  children: React.ReactNode
}

export function ChartStateWrapper({
  title,
  isLoading,
  error,
  isEmpty,
  cardClassName = '@container/card',
  contentClassName,
  emptyMessage,
  children,
}: ChartStateWrapperProps) {
  if (isLoading) {
    return (
      <Card className={cardClassName}>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            {title}
          </CardTitle>
        </CardHeader>
        <CardContent className={contentClassName}>
          <div className="space-y-3">
            <Skeleton className="h-[250px] w-full" />
          </div>
        </CardContent>
      </Card>
    )
  }

  if (error) {
    return (
      <Card className={cardClassName}>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent className={contentClassName}>
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    )
  }

  if (isEmpty) {
    return (
      <Card className={cardClassName}>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent className={contentClassName}>
          <div className="flex h-[250px] w-full items-center justify-center text-muted-foreground">
            <p>{emptyMessage || `No ${title.toLowerCase()} data available`}</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className={cardClassName}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className={contentClassName}>{children}</CardContent>
    </Card>
  )
}
