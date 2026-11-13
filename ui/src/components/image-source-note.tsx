import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { useClusterList } from '@/lib/api'

// ImageSourceNote says where a cluster pulls the images of the components the
// platform installs, so a missing private registry shows before an install.
export function ImageSourceNote({ cluster }: { cluster: string }) {
  const { t } = useTranslation()
  const { data: clusters } = useClusterList()
  const target = clusters?.find((item) => item.name === cluster)
  if (!target) return null
  return (
    <p className="text-xs text-muted-foreground">
      {target.imageRegistry ? (
        <>
          {t('imageSource.registry')}{' '}
          <span className="font-mono break-all">{target.imageRegistry}</span>
        </>
      ) : (
        t('imageSource.public')
      )}
      {' · '}
      <Link to="/settings?tab=clusters" className="underline">
        {t('imageSource.change')}
      </Link>
    </p>
  )
}
