import {
  IconChartBar,
  IconCpu,
  IconDatabase,
  IconLayoutDashboard,
  IconRocket,
  IconRouteAltLeft,
} from '@tabler/icons-react'

export const MODEL_SERVING_ITEMS = [
  {
    url: '/model-serving/overview',
    titleKey: 'nav.workbench',
    Icon: IconLayoutDashboard,
  },
  {
    url: '/model-serving/services',
    titleKey: 'nav.inferenceServices',
    Icon: IconRocket,
  },
  {
    url: '/model-serving/gateway',
    titleKey: 'nav.gateway',
    Icon: IconRouteAltLeft,
  },
  {
    url: '/model-serving/usage',
    titleKey: 'nav.modelUsage',
    Icon: IconChartBar,
  },
  {
    url: '/model-serving/models',
    titleKey: 'nav.modelRegistry',
    Icon: IconDatabase,
  },
  { url: '/model-serving/gpus', titleKey: 'nav.gpuResources', Icon: IconCpu },
]
