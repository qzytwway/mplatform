import { useMemo, useState } from 'react'
import { IconLoader } from '@tabler/icons-react'
import { Link, useSearchParams } from 'react-router-dom'

import { RelatedResources, ResourceType } from '@/types/api'
import { useRelatedResources } from '@/lib/api'
import { getCRDResourcePath, isStandardK8sResource } from '@/lib/k8s'
import {
  getResourceDetailPath,
  getResourceMetadata,
} from '@/lib/resource-catalog'
import { Dialog, DialogTrigger } from '@/components/ui/dialog'
import { ResourceIframeDialogContent } from '@/components/resource-iframe-dialog-content'

import { Column, SimpleTable } from './simple-table'
import { Badge } from './ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'

export function RelatedResourcesTable(props: {
  resource: ResourceType
  name: string
  namespace?: string
}) {
  const { resource, name, namespace } = props

  const { data: relatedResources, isLoading } = useRelatedResources(
    resource,
    name,
    namespace
  )

  const relatedColumns = useMemo(
    (): Column<RelatedResources>[] => [
      {
        header: 'Kind',
        accessor: (rs: RelatedResources) => rs.type,
        align: 'left',
        cell: (value: unknown) => (
          <Badge className="capitalize">{value as string}</Badge>
        ),
      },
      {
        header: 'Name',
        accessor: (rs: RelatedResources) => rs,
        cell: (value: unknown) => {
          const rs = value as RelatedResources
          return <RelatedResourceCell rs={rs} />
        },
      },
    ],
    []
  )

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <IconLoader className="animate-spin mr-2" />
        Loading related...
      </div>
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Related</CardTitle>
      </CardHeader>
      <CardContent>
        <SimpleTable
          data={relatedResources || []}
          columns={relatedColumns}
          emptyMessage="No related found"
        />
      </CardContent>
    </Card>
  )
}

function RelatedResourceCell({ rs }: { rs: RelatedResources }) {
  const [open, setOpen] = useState(false)
  const [searchParams] = useSearchParams()
  const metadata = getResourceMetadata(rs.type)
  const isIframe = searchParams.get('iframe') === 'true'

  const path = useMemo(() => {
    if (isStandardK8sResource(rs.type)) {
      return getResourceDetailPath(
        metadata?.type || rs.type,
        rs.name,
        rs.namespace
      )
    }
    return getCRDResourcePath(rs.type, rs.apiVersion!, rs.namespace, rs.name)
  }, [metadata?.type, rs])

  if (isIframe) {
    return (
      <Link
        to={`${path}?iframe=true`}
        className="font-medium app-link cursor-pointer"
      >
        {rs.name}
      </Link>
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <div className="font-medium app-link cursor-pointer">{rs.name}</div>
      </DialogTrigger>
      <ResourceIframeDialogContent
        title={metadata?.singularLabel || rs.type}
        path={path}
      />
    </Dialog>
  )
}
