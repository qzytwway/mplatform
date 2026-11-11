import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

import { ControlPlaneMode, PrecheckResult } from '@/types/api'
import {
  createProvision,
  precheckProvision,
  useKubernetesVersions,
} from '@/lib/api/provisions'
import { translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { CreateSummary } from '@/components/cluster-provision/create-summary'
import {
  Field,
  Section,
  ToggleRow,
} from '@/components/cluster-provision/form-layout'
import { NodeTable } from '@/components/cluster-provision/node-table'
import { PrecheckResults } from '@/components/cluster-provision/precheck-results'
import { SourcesFields } from '@/components/cluster-provision/sources-fields'
import {
  createDefaultForm,
  toSpec,
  validateForm,
  type ProvisionForm,
} from '@/components/cluster-provision/spec-form'
import { SSHFields } from '@/components/cluster-provision/ssh-fields'
import {
  CLUSTER_LIST_PATH,
  provisionPath,
} from '@/components/cluster-provision/utils'

const CONTROL_PLANE_MODES: ControlPlaneMode[] = [
  'single',
  'kube-vip',
  'external-lb',
]

export function ClusterCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const versionsQuery = useKubernetesVersions()
  const [draft, setDraft] = useState<ProvisionForm>(createDefaultForm)
  const [attempted, setAttempted] = useState(false)
  const [precheck, setPrecheck] = useState<PrecheckResult | null>(null)

  usePageTitle(t('clusterProvision.create.title'))

  const form: ProvisionForm = {
    ...draft,
    kubernetesMinor: draft.kubernetesMinor || versionsQuery.data?.default || '',
  }
  const errors = attempted ? validateForm(form, t) : []
  const selectedVersion = versionsQuery.data?.versions.find(
    (version) => version.minor === form.kubernetesMinor
  )
  const { mode } = form.controlPlane
  const update = (patch: Partial<ProvisionForm>) =>
    setDraft((current) => ({ ...current, ...patch }))

  const precheckMutation = useMutation({
    mutationFn: precheckProvision,
    onSuccess: (result) => {
      setPrecheck(result)
      if (result.passed)
        toast.success(t('clusterProvision.messages.precheckPassed'))
      else toast.error(t('clusterProvision.messages.precheckFailed'))
    },
    onError: (error) => toast.error(translateError(error, t)),
  })

  const createMutation = useMutation({
    mutationFn: createProvision,
    onSuccess: ({ id }) => {
      toast.success(t('clusterProvision.messages.created'))
      void queryClient.invalidateQueries({ queryKey: ['provisions'] })
      navigate(provisionPath(id))
    },
    onError: (error) => toast.error(translateError(error, t)),
  })

  const runIfValid = (action: () => void) => {
    setAttempted(true)
    if (validateForm(form, t).length === 0) action()
  }

  const precheckWarned = precheck?.nodes.some((node) =>
    node.checks.some((check) => check.status === 'warn')
  )

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-bold">
          {t('clusterProvision.create.title')}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('clusterProvision.create.description')}
        </p>
      </div>

      <div className="grid grid-cols-1 items-start gap-4 @5xl/main:grid-cols-[minmax(0,1fr)_18rem]">
        <div className="flex min-w-0 flex-col gap-4">
          <Section title={t('clusterProvision.create.sections.basic')}>
            <div className="rounded-lg border bg-muted/40 px-4 py-3">
              <div className="flex items-center gap-2">
                <span className="font-medium">
                  {t('clusterProvision.create.methodTitle')}
                </span>
                <Badge variant="outline">kubeadm</Badge>
              </div>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('clusterProvision.create.methodDescription')}
              </p>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                id="cluster-name"
                label={`${t('clusterProvision.create.name')} *`}
                hint={t('clusterProvision.create.nameHint')}
              >
                <Input
                  id="cluster-name"
                  value={draft.name}
                  onChange={(e) => update({ name: e.target.value })}
                  placeholder={t('clusterProvision.create.namePlaceholder')}
                  autoComplete="off"
                />
              </Field>
              <Field
                id="kubernetes-version"
                label={`${t('clusterProvision.create.version')} *`}
                hint={
                  selectedVersion?.error
                    ? t('clusterProvision.create.versionError', {
                        error: selectedVersion.error,
                      })
                    : undefined
                }
              >
                <Select
                  value={form.kubernetesMinor}
                  onValueChange={(kubernetesMinor) =>
                    update({ kubernetesMinor })
                  }
                  disabled={!versionsQuery.data}
                >
                  <SelectTrigger id="kubernetes-version" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {versionsQuery.data?.versions.map((version) => (
                      <SelectItem key={version.minor} value={version.minor}>
                        {version.minor} ·{' '}
                        {version.patch ||
                          t('clusterProvision.create.versionUnresolved')}
                        {version.minor === versionsQuery.data.default &&
                          ` (${t('clusterProvision.create.versionRecommended')})`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {versionsQuery.error && (
                  <p className="text-xs text-destructive">
                    {t('clusterProvision.create.versionsLoadFailed')}{' '}
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      className="h-auto p-0 text-xs"
                      disabled={versionsQuery.isFetching}
                      onClick={() => void versionsQuery.refetch()}
                    >
                      {t('common.actions.retry')}
                    </Button>
                  </p>
                )}
              </Field>
            </div>
            <Field
              id="cluster-description"
              label={t('clusterProvision.create.descriptionLabel')}
            >
              <Textarea
                id="cluster-description"
                value={draft.description}
                onChange={(e) => update({ description: e.target.value })}
                placeholder={t(
                  'clusterProvision.create.descriptionPlaceholder'
                )}
                rows={2}
              />
            </Field>
          </Section>

          <Section title={t('clusterProvision.create.sections.nodes')}>
            <p className="text-sm text-muted-foreground">
              {t('clusterProvision.create.nodesHint')}
            </p>
            <div className="space-y-3 rounded-lg border bg-muted/40 p-4">
              <div>
                <p className="text-sm font-medium">
                  {t('clusterProvision.create.sshDefaults')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('clusterProvision.create.sshHint')}
                </p>
              </div>
              <SSHFields
                idPrefix="default-ssh"
                value={draft.ssh}
                onChange={(ssh) => update({ ssh })}
              />
            </div>
            <NodeTable
              nodes={draft.nodes}
              defaultSSH={draft.ssh}
              onChange={(nodes) => update({ nodes })}
            />
          </Section>

          <Section title={t('clusterProvision.create.sections.controlPlane')}>
            <Field
              id="control-plane-mode"
              label={t('clusterProvision.create.controlPlaneMode')}
            >
              <Select
                value={mode}
                onValueChange={(value) =>
                  update({
                    controlPlane: {
                      ...draft.controlPlane,
                      mode: value as ControlPlaneMode,
                    },
                  })
                }
              >
                <SelectTrigger id="control-plane-mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CONTROL_PLANE_MODES.map((value) => (
                    <SelectItem key={value} value={value}>
                      {t(`clusterProvision.controlPlaneModes.${value}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {mode !== 'single' && (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-[minmax(0,1fr)_8rem]">
                <Field
                  id="control-plane-endpoint"
                  label={`${t(
                    mode === 'kube-vip'
                      ? 'clusterProvision.create.vip'
                      : 'clusterProvision.create.lbAddress'
                  )} *`}
                  hint={t(
                    mode === 'kube-vip'
                      ? 'clusterProvision.create.vipHint'
                      : 'clusterProvision.create.lbHint'
                  )}
                >
                  <Input
                    id="control-plane-endpoint"
                    value={draft.controlPlane.endpoint}
                    onChange={(e) =>
                      update({
                        controlPlane: {
                          ...draft.controlPlane,
                          endpoint: e.target.value,
                        },
                      })
                    }
                    placeholder={
                      mode === 'kube-vip' ? '192.168.10.100' : 'lb.example.com'
                    }
                    className="font-mono"
                  />
                </Field>
                <Field
                  id="control-plane-port"
                  label={t('clusterProvision.create.endpointPort')}
                >
                  <Input
                    id="control-plane-port"
                    type="number"
                    min={1}
                    max={65535}
                    value={draft.controlPlane.endpointPort || ''}
                    onChange={(e) =>
                      update({
                        controlPlane: {
                          ...draft.controlPlane,
                          endpointPort: Number(e.target.value),
                        },
                      })
                    }
                  />
                </Field>
              </div>
            )}
          </Section>

          <Section title={t('clusterProvision.create.sections.network')}>
            <div className="space-y-2">
              <Label>{t('clusterProvision.create.cni')}</Label>
              <p className="text-sm font-medium">Calico</p>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              {(['podCIDR', 'serviceCIDR', 'dnsDomain'] as const).map((key) => (
                <Field
                  key={key}
                  id={`network-${key}`}
                  label={t(`clusterProvision.create.${key}`)}
                >
                  <Input
                    id={`network-${key}`}
                    value={draft.network[key]}
                    onChange={(e) =>
                      update({
                        network: { ...draft.network, [key]: e.target.value },
                      })
                    }
                    className="font-mono"
                  />
                </Field>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              {t('clusterProvision.create.networkHint')}
            </p>
          </Section>

          <Section title={t('clusterProvision.create.sections.sources')}>
            <SourcesFields
              value={draft.sources}
              onChange={(sources) => update({ sources })}
            />
          </Section>

          <Section title={t('clusterProvision.create.sections.options')}>
            <ToggleRow
              id="schedulable-control-plane"
              label={t('clusterProvision.create.schedulable')}
              hint={t('clusterProvision.create.schedulableHint')}
              checked={draft.schedulableControlPlane}
              onChange={(schedulableControlPlane) =>
                update({ schedulableControlPlane })
              }
            />
            <ToggleRow
              id="is-default"
              label={t('clusterProvision.create.isDefault')}
              hint={t('clusterProvision.create.isDefaultHint')}
              checked={draft.isDefault}
              onChange={(isDefault) => update({ isDefault })}
            />
          </Section>

          {precheck && (
            <Section title={t('clusterProvision.precheck.title')}>
              <Alert variant={precheck.passed ? 'default' : 'destructive'}>
                <AlertDescription>
                  {precheck.passed
                    ? t(
                        precheckWarned
                          ? 'clusterProvision.precheck.passedWithWarnings'
                          : 'clusterProvision.precheck.passed'
                      )
                    : t('clusterProvision.precheck.failed')}
                </AlertDescription>
              </Alert>
              <PrecheckResults nodes={precheck.nodes} />
            </Section>
          )}

          {errors.length > 0 && (
            <Alert variant="destructive">
              <AlertTitle>{t('clusterProvision.validation.title')}</AlertTitle>
              <AlertDescription>
                <ul className="list-disc space-y-1 pl-4">
                  {errors.map((error) => (
                    <li key={error}>{error}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="ghost" asChild>
              <Link to={CLUSTER_LIST_PATH}>{t('common.actions.cancel')}</Link>
            </Button>
            <Button
              variant="outline"
              disabled={precheckMutation.isPending}
              onClick={() =>
                runIfValid(() => precheckMutation.mutate(toSpec(form)))
              }
            >
              {precheckMutation.isPending
                ? t('clusterProvision.actions.prechecking')
                : t('clusterProvision.actions.precheck')}
            </Button>
            <Button
              disabled={createMutation.isPending}
              onClick={() =>
                runIfValid(() => createMutation.mutate(toSpec(form)))
              }
            >
              {createMutation.isPending
                ? t('clusterProvision.actions.submitting')
                : t('clusterProvision.actions.create')}
            </Button>
          </div>
        </div>

        <CreateSummary form={form} version={selectedVersion} />
      </div>
    </div>
  )
}
