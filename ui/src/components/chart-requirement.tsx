import { useTranslation } from 'react-i18next'

import type { ComponentChart, ComponentImage } from '@/lib/api/shared'
import { Badge } from '@/components/ui/badge'

interface ChartRequirementProps {
  chart: ComponentChart
  // Shown when the chart is not in the local chart repository.
  missingLabel: string
}

// ChartRequirement shows a chart a component installs, whether its version is
// the one the platform is verified with, and the images an offline registry
// must provide for it.
export function ChartRequirement({
  chart,
  missingLabel,
}: ChartRequirementProps) {
  const { t } = useTranslation()
  const untested =
    !!chart.version &&
    !!chart.testedVersion &&
    chart.version !== chart.testedVersion
  return (
    <div className="space-y-1 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono">{chart.name}</span>
        {chart.version ? (
          <Badge variant="outline">{chart.version}</Badge>
        ) : (
          <Badge variant="destructive">{missingLabel}</Badge>
        )}
        {untested && (
          <Badge
            variant="outline"
            className="border-warning/50 text-warning"
            title={t('chartRequirement.untestedHint')}
          >
            {t('chartRequirement.untested', { version: chart.testedVersion })}
          </Badge>
        )}
      </div>
      {chart.error ? (
        <p className="break-all text-destructive">
          {t('chartRequirement.imagesError', { error: chart.error })}
        </p>
      ) : (
        chart.images &&
        chart.images.length > 0 && (
          <ul className="space-y-0.5">
            {chart.images.map((image) => (
              <li
                key={image}
                className="font-mono break-all text-muted-foreground"
              >
                {image}
              </li>
            ))}
          </ul>
        )
      )}
    </div>
  )
}

interface ImageRequirementProps {
  item: ComponentImage
  // What the component deploys the image for.
  label: string
}

// ImageRequirement shows an image a component deploys without a chart.
export function ImageRequirement({ item, label }: ImageRequirementProps) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <span className="font-mono">{item.name}</span>
      <Badge variant="outline">{label}</Badge>
      <span className="font-mono break-all text-muted-foreground">
        {item.image}
      </span>
    </div>
  )
}
