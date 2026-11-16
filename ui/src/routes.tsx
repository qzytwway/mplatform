import { createBrowserRouter, Navigate } from 'react-router-dom'

import App, { StandaloneAIChatApp } from './App'
import { HomeRoute } from './components/home-route'
import { InitCheckRoute } from './components/init-check-route'
import { ProtectedRoute } from './components/protected-route'
import { MODEL_SERVING_PATHS } from './lib/model-serving-paths'
import { getSubPath } from './lib/subpath'
import { CRListPage } from './pages/cr-list-page'
import { HelmChartDetailPage } from './pages/helm-chart-detail-page'
import { HelmChartListPage } from './pages/helm-chart-list-page'
import { InitializationPage } from './pages/initialization'
import { LoginPage } from './pages/login'
import { CallerCreatePage } from './pages/model-serving/caller-create-page'
import { CallerEditPage } from './pages/model-serving/caller-edit-page'
import { CallerQuotaPage } from './pages/model-serving/caller-quota-page'
import { GatewayPage } from './pages/model-serving/gateway-page'
import { GPUsPage } from './pages/model-serving/gpus-page'
import { ModelCreatePage } from './pages/model-serving/model-create-page'
import { ModelEditPage } from './pages/model-serving/model-edit-page'
import { ModelsPage } from './pages/model-serving/models-page'
import { ServiceCreatePage } from './pages/model-serving/service-create-page'
import { ServiceDetailPage } from './pages/model-serving/service-detail-page'
import { ServiceEditPage } from './pages/model-serving/service-edit-page'
import { ServiceScalePage } from './pages/model-serving/service-scale-page'
import { ServicesPage } from './pages/model-serving/services-page'
import { StorageConfigPage } from './pages/model-serving/storage-config-page'
import { UsagePage } from './pages/model-serving/usage-page'
import { WorkbenchPage } from './pages/model-serving/workbench-page'
import { AlertHistoryPage } from './pages/observability/alert-history-page'
import { AlertsPage } from './pages/observability/alerts-page'
import { LogsPage } from './pages/observability/logs-page'
import { MetricsDashboardPage } from './pages/observability/metrics-dashboard-page'
import { MetricsPage } from './pages/observability/metrics-page'
import { Overview } from './pages/overview'
import { ClusterCreatePage } from './pages/platform/cluster-create-page'
import { ClusterListPage } from './pages/platform/cluster-list-page'
import { ProvisionDetailPage } from './pages/platform/provision-detail-page'
import { ResourceDetail } from './pages/resource-detail'
import { ResourceList } from './pages/resource-list'
import { SettingsPage } from './pages/settings'

const subPath = getSubPath()

export const router = createBrowserRouter(
  [
    {
      path: '/setup',
      element: <InitializationPage />,
    },
    {
      path: '/login',
      element: (
        <InitCheckRoute allowIncompleteSetup>
          <LoginPage />
        </InitCheckRoute>
      ),
    },
    {
      path: '/ai-chat-box',
      element: (
        <InitCheckRoute>
          <ProtectedRoute>
            <StandaloneAIChatApp />
          </ProtectedRoute>
        </InitCheckRoute>
      ),
    },
    {
      path: '/',
      element: (
        <InitCheckRoute>
          <ProtectedRoute>
            <App />
          </ProtectedRoute>
        </InitCheckRoute>
      ),
      children: [
        {
          index: true,
          element: <HomeRoute />,
        },
        {
          path: 'dashboard',
          element: <Overview />,
        },
        {
          path: 'settings',
          element: <SettingsPage />,
        },
        {
          path: 'platform/clusters',
          element: <ClusterListPage />,
        },
        {
          path: 'platform/clusters/create',
          element: <ClusterCreatePage />,
        },
        {
          path: 'platform/clusters/provisions/:id',
          element: <ProvisionDetailPage />,
        },
        {
          path: 'observability/metrics',
          element: <MetricsPage />,
        },
        {
          path: 'observability/metrics/:uid',
          element: <MetricsDashboardPage />,
        },
        {
          path: 'observability/logs',
          element: <LogsPage />,
        },
        {
          path: 'observability/alerts',
          element: <AlertsPage view="active" />,
        },
        {
          path: 'observability/alerts/history',
          element: <AlertHistoryPage />,
        },
        {
          path: 'observability/alerts/rules',
          element: <AlertsPage view="rules" />,
        },
        {
          path: 'observability/alerts/silences',
          element: <AlertsPage view="silences" />,
        },
        {
          path: 'observability/alerts/notifications',
          element: <AlertsPage view="notifications" />,
        },
        {
          path: MODEL_SERVING_PATHS.root,
          element: <Navigate to={MODEL_SERVING_PATHS.overview} replace />,
        },
        {
          path: MODEL_SERVING_PATHS.overview,
          element: <WorkbenchPage />,
        },
        {
          path: MODEL_SERVING_PATHS.services,
          element: <ServicesPage />,
        },
        {
          path: MODEL_SERVING_PATHS.serviceCreate,
          element: <ServiceCreatePage />,
        },
        {
          path: MODEL_SERVING_PATHS.service(':namespace', ':name'),
          element: <ServiceDetailPage />,
        },
        {
          path: MODEL_SERVING_PATHS.serviceEdit(':namespace', ':name'),
          element: <ServiceEditPage />,
        },
        {
          path: MODEL_SERVING_PATHS.serviceScale(':namespace', ':name'),
          element: <ServiceScalePage />,
        },
        {
          path: MODEL_SERVING_PATHS.gateway,
          element: <GatewayPage />,
        },
        {
          path: MODEL_SERVING_PATHS.callerCreate,
          element: <CallerCreatePage />,
        },
        {
          path: MODEL_SERVING_PATHS.callerEdit(':id'),
          element: <CallerEditPage />,
        },
        {
          path: MODEL_SERVING_PATHS.callerQuota(':id'),
          element: <CallerQuotaPage />,
        },
        {
          path: MODEL_SERVING_PATHS.usage,
          element: <UsagePage />,
        },
        {
          path: MODEL_SERVING_PATHS.models,
          element: <ModelsPage />,
        },
        {
          path: MODEL_SERVING_PATHS.modelCreate,
          element: <ModelCreatePage />,
        },
        {
          path: MODEL_SERVING_PATHS.modelEdit(':id'),
          element: <ModelEditPage />,
        },
        {
          path: MODEL_SERVING_PATHS.storage,
          element: <StorageConfigPage />,
        },
        {
          path: MODEL_SERVING_PATHS.gpus,
          element: <GPUsPage />,
        },
        {
          path: 'crds/:crd',
          element: <CRListPage />,
        },
        {
          path: 'charts',
          element: <HelmChartListPage />,
        },
        {
          path: 'charts/:repository/:name',
          element: <HelmChartDetailPage />,
        },
        {
          path: 'crds/:resource/:namespace/:name',
          element: <ResourceDetail />,
        },
        {
          path: 'crds/:resource/:name',
          element: <ResourceDetail />,
        },
        {
          path: ':resource/:name',
          element: <ResourceDetail />,
        },
        {
          path: ':resource',
          element: <ResourceList />,
        },
        {
          path: ':resource/:namespace/:name',
          element: <ResourceDetail />,
        },
      ],
    },
  ],
  {
    basename: subPath,
  }
)
