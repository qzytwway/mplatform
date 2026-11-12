import { useState } from 'react'
import { IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import { translateError } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Section } from '@/components/form-layout'
import {
  CallerForm,
  type IssuedKey,
} from '@/components/model-serving/caller-form'
import {
  CallerPageLayout,
  GATEWAY_PATH,
} from '@/components/model-serving/caller-page-layout'

export function CallerCreatePage() {
  const { t } = useTranslation()
  // The new key lives only in this page's state.
  const [issuedKey, setIssuedKey] = useState<IssuedKey>()

  return (
    <CallerPageLayout
      title={t('modelServing.callers.createTitle')}
      description={t('modelServing.callers.description')}
    >
      {issuedKey ? (
        <div className="flex max-w-4xl flex-col gap-4">
          <Section
            title={t('modelServing.callers.newKeyTitle', {
              name: issuedKey.name,
            })}
          >
            <p className="text-sm text-muted-foreground">
              {t('modelServing.callers.newKeyDescription')}
            </p>
            <div className="flex items-center gap-1">
              <code className="min-w-0 flex-1 rounded bg-muted px-2 py-2 font-mono text-sm break-all">
                {issuedKey.apiKey}
              </code>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={t('common.actions.copy')}
                onClick={() =>
                  navigator.clipboard.writeText(issuedKey.apiKey).then(
                    () => toast.success(t('common.messages.copied')),
                    (err) => toast.error(translateError(err, t))
                  )
                }
              >
                <IconCopy className="h-4 w-4" />
              </Button>
            </div>
          </Section>
          <div className="flex justify-end">
            <Button asChild>
              <Link to={GATEWAY_PATH}>{t('common.actions.done')}</Link>
            </Button>
          </div>
        </div>
      ) : (
        <CallerForm caller={null} onSaved={setIssuedKey} />
      )}
    </CallerPageLayout>
  )
}
