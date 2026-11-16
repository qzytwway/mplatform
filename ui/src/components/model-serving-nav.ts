import {
  IconChartBar,
  IconCpu,
  IconDatabase,
  IconLayoutDashboard,
  IconRocket,
  IconRouteAltLeft,
} from '@tabler/icons-react'

import { MODEL_SERVING_PATHS } from '@/lib/model-serving-paths'

export const MODEL_SERVING_ITEMS = [
  {
    url: MODEL_SERVING_PATHS.overview,
    titleKey: 'nav.workbench',
    Icon: IconLayoutDashboard,
  },
  {
    url: MODEL_SERVING_PATHS.services,
    titleKey: 'nav.inferenceServices',
    Icon: IconRocket,
  },
  {
    url: MODEL_SERVING_PATHS.gateway,
    titleKey: 'nav.gateway',
    Icon: IconRouteAltLeft,
  },
  {
    url: MODEL_SERVING_PATHS.usage,
    titleKey: 'nav.modelUsage',
    Icon: IconChartBar,
  },
  {
    url: MODEL_SERVING_PATHS.models,
    titleKey: 'nav.modelRegistry',
    Icon: IconDatabase,
  },
  {
    url: MODEL_SERVING_PATHS.gpus,
    titleKey: 'nav.gpuResources',
    Icon: IconCpu,
  },
]
