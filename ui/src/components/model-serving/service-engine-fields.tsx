import { useTranslation } from 'react-i18next'

import type {
  DeploymentMode,
  Engine,
  ModelType,
  ParserSuggestion,
} from '@/lib/api/model-serving'
import { Input } from '@/components/ui/input'
import { HintLabel } from '@/components/form-layout'

import type { ServiceFormValues, ServiceModeValues } from './service-form'
import { parallelism } from './service-modes'

type EngineValues = Pick<
  ServiceFormValues,
  | 'contextLength'
  | 'maxRunningRequests'
  | 'gpuMemoryFraction'
  | 'toolCallParser'
  | 'reasoningParser'
>

interface ServiceEngineFieldsProps {
  engine: Engine
  mode: DeploymentMode
  modelType: ModelType | undefined
  gpuCount: string
  modeValues: ServiceModeValues
  values: EngineValues
  onValuesChange: (patch: Partial<EngineValues>) => void
}

// ServiceEngineFields sets the engine settings beginners most often need,
// which the backend passes under each Engine's flags, and shows the
// parallelism the GPU counts give.
export function ServiceEngineFields({
  engine,
  mode,
  modelType,
  gpuCount,
  modeValues,
  values,
  onValuesChange,
}: ServiceEngineFieldsProps) {
  const { t } = useTranslation()
  const engineDefault = t('modelServing.services.engine.engineDefault')

  const formatParallelism = (gpus: string, nodes: string) => {
    const { tp, pp } = parallelism(engine, Number(gpus), Number(nodes))
    return pp > 1 ? `TP ${tp} × PP ${pp}` : `TP ${tp}`
  }
  const parallelismText =
    mode === 'pd'
      ? (['prefill', 'decode'] as const)
          .map(
            (role) =>
              `${t(`modelServing.roles.${role}`)} ${formatParallelism(
                modeValues[role].gpuCount,
                modeValues[role].groupSize
              )}`
          )
          .join(' · ')
      : formatParallelism(
          gpuCount,
          mode === 'multi' ? modeValues.groupSize : '1'
        )

  const parserField = (
    field: 'toolCallParser' | 'reasoningParser',
    suggestions: ParserSuggestion[]
  ) => {
    const id = `inference-${field}`
    return (
      <div className="col-span-2 space-y-2">
        <HintLabel
          htmlFor={id}
          hint={t(`modelServing.services.engine.${field}Hint`)}
        >
          {t(`modelServing.services.engine.${field}`)}
        </HintLabel>
        <Input
          id={id}
          list={`${id}-suggestions`}
          value={values[field]}
          onChange={(event) => onValuesChange({ [field]: event.target.value })}
          placeholder={t('modelServing.services.engine.parserOff')}
          pattern="[A-Za-z0-9_.\-]*"
          className="font-mono"
        />
        <datalist id={`${id}-suggestions`}>
          {suggestions.map((item) => (
            <option
              key={item.value}
              value={item.value}
              label={
                item.models || t('modelServing.services.engine.parserAuto')
              }
            />
          ))}
        </datalist>
      </div>
    )
  }

  return (
    <>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-context-length"
          hint={t('modelServing.services.engine.contextLengthHint')}
        >
          {t('modelServing.services.engine.contextLength')}
        </HintLabel>
        <Input
          id="inference-context-length"
          type="number"
          min={1}
          value={values.contextLength}
          onChange={(event) =>
            onValuesChange({ contextLength: event.target.value })
          }
          placeholder={engineDefault}
        />
      </div>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-max-running-requests"
          hint={t('modelServing.services.engine.maxRunningRequestsHint')}
        >
          {t('modelServing.services.engine.maxRunningRequests')}
        </HintLabel>
        <Input
          id="inference-max-running-requests"
          type="number"
          min={1}
          value={values.maxRunningRequests}
          onChange={(event) =>
            onValuesChange({ maxRunningRequests: event.target.value })
          }
          placeholder={engineDefault}
        />
      </div>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-gpu-memory-fraction"
          hint={t('modelServing.services.engine.gpuMemoryFractionHint')}
        >
          {t('modelServing.services.engine.gpuMemoryFraction')}
        </HintLabel>
        <Input
          id="inference-gpu-memory-fraction"
          type="number"
          min={0.05}
          max={1}
          step={0.01}
          value={values.gpuMemoryFraction}
          onChange={(event) =>
            onValuesChange({ gpuMemoryFraction: event.target.value })
          }
          placeholder={engineDefault}
        />
      </div>
      <div className="space-y-2">
        <HintLabel
          htmlFor="inference-parallelism"
          hint={t('modelServing.services.engine.parallelismHint')}
        >
          {t('modelServing.services.engine.parallelism')}
        </HintLabel>
        <Input
          id="inference-parallelism"
          value={parallelismText}
          readOnly
          className="font-mono text-muted-foreground"
        />
      </div>
      {/* Embedding models neither call tools nor reason. */}
      {modelType !== 'embedding' && (
        <>
          {parserField('toolCallParser', engine.toolCallParsers)}
          {parserField('reasoningParser', engine.reasoningParsers)}
        </>
      )}
    </>
  )
}
