import type { ReactNode } from 'react'
import { ArrowLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'
import { usePageTitle } from '@/hooks/use-page-title'

interface CallerPageLayoutProps {
  title: string
  description?: string
  children: ReactNode
}

export function CallerPageLayout({
  title,
  description,
  children,
}: CallerPageLayoutProps) {
  const { t } = useTranslation()
  usePageTitle(title)

  return (
    <div className="flex flex-col gap-4">
      <Link
        to={MODEL_SERVING_PATHS.gateway}
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {t('modelServing.gateway.title')}
      </Link>
      <div>
        <h1 className="text-2xl font-bold">{title}</h1>
        {description && (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      {children}
    </div>
  )
}
