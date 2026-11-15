import { useTranslation } from 'react-i18next'

import type {
  DeploymentMode,
  InferenceServicePreview,
} from '@/lib/api/model-serving'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { YamlFileTreeViewerNative } from '@/components/yaml-file-tree-viewer-native'

import { ServiceDeploymentCard } from './service-deployment-card'

// ServicePreviewDialog shows what creating a service would make: how each
// role starts, as the service detail shows it, and the YAML of every object.
export function ServicePreviewDialog({
  preview,
  mode,
  onClose,
}: {
  preview: InferenceServicePreview | null
  mode: DeploymentMode
  onClose: () => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={!!preview} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="flex h-[calc(100dvh-4rem)] w-[calc(100vw-4rem)] !max-w-[calc(100vw-4rem)] flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t('modelServing.services.preview.title')}</DialogTitle>
          <DialogDescription>
            {t('modelServing.services.preview.description')}
          </DialogDescription>
        </DialogHeader>
        {preview && (
          <Tabs defaultValue="args" className="min-h-0 flex-1 gap-3">
            <TabsList>
              <TabsTrigger value="args">
                {t('modelServing.services.preview.args')}
              </TabsTrigger>
              <TabsTrigger value="yaml">
                {t('modelServing.services.preview.yaml')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="args" className="min-h-0 overflow-y-auto pr-1">
              <ServiceDeploymentCard config={preview.deployment} mode={mode} />
            </TabsContent>
            <TabsContent value="yaml" className="flex min-h-0 flex-col">
              <YamlFileTreeViewerNative
                files={preview.resources}
                title={t('modelServing.services.preview.yaml')}
                emptyMessage={t('modelServing.services.preview.empty')}
                fillHeight
              />
            </TabsContent>
          </Tabs>
        )}
      </DialogContent>
    </Dialog>
  )
}
