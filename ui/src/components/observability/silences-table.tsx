import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { expireSilence, Silence, SilenceMatcher } from '@/lib/api/alerts'
import { cn, formatDate } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const STATE_STYLES: Record<string, string> = {
  active: 'bg-warning/15 text-warning',
  pending: 'bg-info/15 text-info',
  expired: 'bg-muted text-muted-foreground',
}

function matcherText(matcher: SilenceMatcher) {
  const negated = matcher.isEqual === false
  const operator = matcher.isRegex
    ? negated
      ? '!~'
      : '=~'
    : negated
      ? '!='
      : '='
  return `${matcher.name}${operator}"${matcher.value}"`
}

export function SilencesTable({ silences }: { silences: Silence[] }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showExpired, setShowExpired] = useState(false)
  const [expiring, setExpiring] = useState<Silence | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const visible = silences
    .filter((silence) => showExpired || silence.status?.state !== 'expired')
    .sort((a, b) => b.startsAt.localeCompare(a.startsAt))

  const confirmExpire = async () => {
    if (!expiring) return
    setSubmitting(true)
    try {
      await expireSilence(expiring.id)
      toast.success(t('observability.alerts.silences.expired'))
      await queryClient.invalidateQueries({ queryKey: ['alert-silences'] })
      setExpiring(null)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('observability.alerts.silences.expireFailed')
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Switch
          id="show-expired-silences"
          checked={showExpired}
          onCheckedChange={setShowExpired}
        />
        <Label htmlFor="show-expired-silences">
          {t('observability.alerts.silences.showExpired')}
        </Label>
      </div>
      <div className="overflow-hidden rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('common.fields.status')}</TableHead>
              <TableHead>
                {t('observability.alerts.silence.matchers')}
              </TableHead>
              <TableHead>{t('observability.alerts.silences.window')}</TableHead>
              <TableHead>
                {t('observability.alerts.silences.createdBy')}
              </TableHead>
              <TableHead>{t('observability.alerts.silence.comment')}</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="py-10 text-center text-muted-foreground"
                >
                  {t('observability.alerts.silences.empty')}
                </TableCell>
              </TableRow>
            ) : (
              visible.map((silence) => {
                const state = silence.status?.state ?? 'active'
                return (
                  <TableRow key={silence.id}>
                    <TableCell>
                      <span
                        className={cn(
                          'inline-flex h-5 items-center rounded-full px-2 text-xs font-medium',
                          STATE_STYLES[state]
                        )}
                      >
                        {t(`observability.alerts.silences.state.${state}`)}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-md whitespace-normal">
                      <div className="flex flex-wrap gap-1.5">
                        {silence.matchers.map((matcher) => (
                          <span
                            key={matcherText(matcher)}
                            className="rounded-full border px-2 py-0.5 font-mono text-xs break-all"
                          >
                            {matcherText(matcher)}
                          </span>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs tabular-nums text-muted-foreground">
                      {formatDate(silence.startsAt)}
                      <br />
                      {formatDate(silence.endsAt)}
                    </TableCell>
                    <TableCell>{silence.createdBy}</TableCell>
                    <TableCell className="max-w-xs whitespace-normal text-sm">
                      {silence.comment}
                    </TableCell>
                    <TableCell>
                      {state !== 'expired' && (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setExpiring(silence)}
                        >
                          {t('observability.alerts.silences.expire')}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog
        open={!!expiring}
        onOpenChange={(open) => !open && setExpiring(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t('observability.alerts.silences.confirmTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('observability.alerts.silences.confirmDescription')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExpiring(null)}>
              {t('common.actions.cancel')}
            </Button>
            <Button onClick={confirmExpire} disabled={submitting}>
              {t('observability.alerts.silences.expire')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
