import { createBrowserRouter } from 'react-router-dom'

import App, { StandaloneAIChatApp } from './App'
import { InitCheckRoute } from './components/init-check-route'
import { ProtectedRoute } from './components/protected-route'
import { getSubPath } from './lib/subpath'
import { CRListPage } from './pages/cr-list-page'
import { HelmChartDetailPage } from './pages/helm-chart-detail-page'
import { HelmChartListPage } from './pages/helm-chart-list-page'
import { InitializationPage } from './pages/initialization'
import { LoginPage } from './pages/login'
import { GatewayPage } from './pages/model-serving/gateway-page'
import { GPUsPage } from './pages/model-serving/gpus-page'
import { ModelsPage } from './pages/model-serving/models-page'
import { ServiceDetailPage } from './pages/model-serving/service-detail-page'
import { ServicesPage } from './pages/model-serving/services-page'
import { UsagePage } from './pages/model-serving/usage-page'
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
          element: <Overview />,
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
          element: <AlertsPage />,
        },
        {
          path: 'model-serving/services',
          element: <ServicesPage />,
        },
        {
          path: 'model-serving/services/:namespace/:name',
          element: <ServiceDetailPage />,
        },
        {
          path: 'model-serving/gateway',
          element: <GatewayPage />,
        },
        {
          path: 'model-serving/usage',
          element: <UsagePage />,
        },
        {
          path: 'model-serving/models',
          element: <ModelsPage />,
        },
        {
          path: 'model-serving/gpus',
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
