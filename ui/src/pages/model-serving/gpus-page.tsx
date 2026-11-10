import { useTranslation } from 'react-i18next'

import { useGPUNodes } from '@/lib/api/model-serving'
import { translateError } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

export function GPUsPage() {
  const { t } = useTranslation()
  const { data, isLoading, error } = useGPUNodes()

  const renderContent = () => {
    if (isLoading) return <Skeleton className="h-64 w-full" />
    if (error || !data)
      return (
        <p className="text-sm text-destructive">{translateError(error, t)}</p>
      )
    if (data.items.length === 0)
      return (
        <div className="rounded-xl border border-dashed px-6 py-14 text-center">
          <h2 className="text-base font-semibold">
            {t('modelServing.gpus.empty')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('modelServing.gpus.emptyHint')}
          </p>
        </div>
      )
    return (
      <div className="rounded-md border">
        <Table>
          <TableHeader className="bg-muted">
            <TableRow>
              <TableHead>{t('common.fields.node')}</TableHead>
              <TableHead>{t('modelServing.gpus.product')}</TableHead>
              <TableHead>{t('common.fields.capacity')}</TableHead>
              <TableHead>{t('common.fields.allocatable')}</TableHead>
              <TableHead>{t('modelServing.gpus.allocated')}</TableHead>
              <TableHead>{t('common.fields.status')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.items.map((node) => (
              <TableRow key={node.name}>
                <TableCell className="font-medium">{node.name}</TableCell>
                <TableCell>{node.product || '-'}</TableCell>
                <TableCell className="tabular-nums">{node.capacity}</TableCell>
                <TableCell className="tabular-nums">
                  {node.allocatable}
                </TableCell>
                <TableCell className="tabular-nums">{node.allocated}</TableCell>
                <TableCell>
                  {node.ready ? (
                    <Badge variant="outline">{t('common.fields.ready')}</Badge>
                  ) : (
                    <Badge variant="destructive">
                      {t('common.messages.notReady')}
                    </Badge>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">{t('modelServing.gpus.title')}</h1>
      {renderContent()}
    </div>
  )
}
